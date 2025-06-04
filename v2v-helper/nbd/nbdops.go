// Copyright © 2024 The vjailbreak authors

package nbd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils"

	"golang.org/x/sys/unix"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	"libguestfs.org/libnbd"
)

//go:generate mockgen -source=../nbd/nbdops.go -destination=../nbd/nbdops_mock.go -package=nbd

type NBDOperations interface {
	StartNBDServer(vm *object.VirtualMachine, server, username, password, thumbprint, snapref, file string, progchan chan string) error
	StopNBDServer() error
	CopyDisk(ctx context.Context, dest string, diskindex int) error
	CopyChangedBlocks(ctx context.Context, changedAreas types.DiskChangeInfo, path string) error
}

type NBDServer struct {
	cmd          *exec.Cmd
	tmp_dir      string
	progresschan chan string
}

type BlockStatusData struct {
	Offset int64
	Length int64
	Flags  uint32
}

const MaxChunkSize = 64 * 1024 * 1024

// MaxBlockStatusLength limits the maximum block status request size to 2GB
const MaxBlockStatusLength = (2 << 30)

// MaxPreadLengthESX limits individual VDDK data block transfers to 23MB.
// Larger block sizes fail immediately.
const MaxPreadLengthESX = (23 << 20)

// MaxPreadLengthVC limits indidivual VDDK data block transfers to 2MB only when
// connecting to vCenter. With vCenter endpoints, multiple simultaneous importer
// pods with larger read sizes cause allocation failures on the server, and the
// imports start to fail:
//
//	"NfcFssrvrProcessErrorMsg: received NFC error 5 from server:
//	 Failed to allocate the requested 24117272 bytes"
const MaxPreadLengthVC = (2 << 20)

// MaxPreadLength is the maxmimum read size to request from VMware. Default to
// the larger option, and reduce it in createVddkDataSource when connecting to
// vCenter endpoints.
var MaxPreadLength = MaxPreadLengthESX

// Request blocks one at a time from libnbd
var fixedOptArgs = libnbd.BlockStatusOptargs{
	Flags:    libnbd.CMD_FLAG_REQ_ONE,
	FlagsSet: true,
}

func (nbdserver *NBDServer) StartNBDServer(vm *object.VirtualMachine, server, username, password, thumbprint, snapref, file string, progchan chan string) error {
	tmp_dir, err := os.MkdirTemp("", "nbdkit-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	// Create the configuration file
	// Ref: https://tecblog.au.de/veeam-v12-12-1-double-your-nbd-backup-performance/
	configFile := "/home/fedora/vddk.conf"
	configContent := `vixDiskLib.nfcAio.Session.BufSizeIn64KB=16
vixDiskLib.nfcAio.Session.BufCount=4`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		return err
	}

	socket := fmt.Sprintf("%s/nbdkit.sock", tmp_dir)
	pidFile := fmt.Sprintf("%s/nbdkit.pid", tmp_dir)

	cmd := exec.Command(
		"nbdkit",
		"--exit-with-parent",
		"--readonly",
		"--foreground",
		fmt.Sprintf("--unix=%s", socket),
		fmt.Sprintf("--pidfile=%s", pidFile),
		"--verbose",
		"-D vddk.datapath=0",
		"-D nbdkit.backend.datapath=0",
		"vddk",
		"libdir=/home/fedora/vmware-vix-disklib-distrib",
		fmt.Sprintf("server=%s", server),
		fmt.Sprintf("user=%s", username),
		fmt.Sprintf("password=%s", password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		"compression=fastlz",
		"config=/home/fedora/vddk.conf",
		"transports=file:nbdssl:nbd",
		fmt.Sprintf("vm=moref=%s", vm.Reference().Value),
		fmt.Sprintf("snapshot=%s", snapref),
		file,
	)

	// Log the command
	cmdstring := ""
	for _, arg := range cmd.Args {

		if strings.Contains(arg, password) {
			cmdstring += "password=[REDACTED] "
		} else {
			cmdstring += fmt.Sprintf("%s ", arg)
		}
	}

	utils.AddDebugOutputToFile(cmd)

	utils.PrintLog(fmt.Sprintf("Executing %s\n", cmdstring))
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start nbdkit: %v", err)
	}
	nbdserver.cmd = cmd
	nbdserver.tmp_dir = tmp_dir
	nbdserver.progresschan = progchan
	return nil
}

func (nbdserver *NBDServer) StopNBDServer() error {
	err := nbdserver.cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("failed to kill nbdkit: %v", err)
	}
	os.RemoveAll(nbdserver.tmp_dir)
	return nil
}

func (nbdserver *NBDServer) CopyDisk(ctx context.Context, dest string, diskindex int) error {
	// Copy the disk from source to destination
	progressRead, progressWrite, err := os.Pipe()
	if err != nil {
		return errors.Wrapf(err, "failed to create pipe")
	}
	defer progressRead.Close()
	defer progressWrite.Close()

	cmd := exec.CommandContext(ctx, "nbdcopy", "--progress=3", "--target-is-zero", generateSockUrl(nbdserver.tmp_dir), dest)
	cmd.ExtraFiles = []*os.File{progressWrite}

	utils.PrintLog(fmt.Sprintf("Executing %s\n", cmd.String()))
	go func() {
		scanner := bufio.NewScanner(progressRead)
		lastProgress := 0
		for scanner.Scan() {
			progressInt, _, err := utils.ParseFraction(scanner.Text())
			if err != nil {
				utils.PrintLog(fmt.Sprintf("Error converting progress percent to int: %v", err))
				continue
			}
			msg := fmt.Sprintf("Copying disk %d, Completed: %d%%", diskindex, progressInt)
			utils.PrintLog(msg)

			if lastProgress <= progressInt-10 {
				nbdserver.progresschan <- msg
				lastProgress = progressInt
			}
		}
	}()
	utils.AddDebugOutputToFile(cmd)
	err = cmd.Run()
	if err != nil {
		// retry once with debug enabled, to get more details
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "failed to run nbdcopy")
		}
	}
	return nil
}

func getBlockStatus(handle *libnbd.Libnbd, extent types.DiskChangeExtent) []*BlockStatusData {
	var blocks []*BlockStatusData

	// Callback for libnbd.BlockStatus. Needs to modify blocks list above.
	updateBlocksCallback := func(metacontext string, nbdOffset uint64, extents []uint32, err *int) int {
		if nbdOffset > math.MaxInt64 {
			utils.PrintLog(fmt.Sprintf("Block status offset too big for conversion: 0x%x", nbdOffset))
			return -2
		}
		offset := int64(nbdOffset)

		if *err != 0 {
			utils.PrintLog(fmt.Sprintf("Block status callback error at offset %d: error code %d", offset, *err))
			return *err
		}
		if metacontext != "base:allocation" {
			utils.PrintLog(fmt.Sprintf("Offset %d not base:allocation, ignoring", offset))
			return 0
		}
		if (len(extents) % 2) != 0 {
			utils.PrintLog(fmt.Sprintf("Block status entry at offset %d has unexpected length %d!", offset, len(extents)))
			return -1
		}
		for i := 0; i < len(extents); i += 2 {
			length, flags := int64(extents[i]), extents[i+1]
			if blocks != nil {
				last := len(blocks) - 1
				lastBlock := blocks[last]
				lastFlags := lastBlock.Flags
				lastOffset := lastBlock.Offset + lastBlock.Length
				if lastFlags == flags && lastOffset == offset {
					// Merge with previous block
					blocks[last] = &BlockStatusData{
						Offset: lastBlock.Offset,
						Length: lastBlock.Length + length,
						Flags:  lastFlags,
					}
				} else {
					blocks = append(blocks, &BlockStatusData{Offset: offset, Length: length, Flags: flags})
				}
			} else {
				blocks = append(blocks, &BlockStatusData{Offset: offset, Length: length, Flags: flags})
			}
			offset += length
		}
		return 0
	}

	if extent.Length < 1024*1024 {
		blocks = append(blocks, &BlockStatusData{
			Offset: extent.Start,
			Length: extent.Length,
			Flags:  0})
		return blocks
	}

	lastOffset := extent.Start
	endOffset := extent.Start + extent.Length
	for lastOffset < endOffset {
		var length int64
		missingLength := endOffset - lastOffset
		if missingLength > (MaxBlockStatusLength) {
			length = MaxBlockStatusLength
		} else {
			length = missingLength
		}
		createWholeBlock := func() []*BlockStatusData {
			block := &BlockStatusData{
				Offset: extent.Start,
				Length: extent.Length,
				Flags:  0,
			}
			blocks = []*BlockStatusData{block}
			return blocks
		}
		err := handle.BlockStatus(uint64(length), uint64(lastOffset), updateBlocksCallback, &fixedOptArgs)
		if err != nil {
			utils.PrintLog(fmt.Sprintf("Error getting block status at offset %d, returning whole block instead. Error was: %v", lastOffset, err))
			return createWholeBlock()
		}
		last := len(blocks) - 1
		newOffset := blocks[last].Offset + blocks[last].Length
		if lastOffset == newOffset {
			utils.PrintLog(fmt.Sprintf("No new block status data at offset %d, returning whole block.", newOffset))
			return createWholeBlock()
		}
		lastOffset = newOffset
	}

	return blocks
}

// pwrite writes the given byte buffer to the sink at the given offset
func pwrite(fd *os.File, buffer []byte, offset uint64) (int, error) {
	blocksize := len(buffer)
	written, err := syscall.Pwrite(int(fd.Fd()), buffer, int64(offset))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to write %d bytes at offset %d", blocksize, offset)
	}
	if written < blocksize {
		utils.PrintLog(fmt.Sprintf("Wrote less than blocksize (%d): %d", blocksize, written))
	}
	return written, nil
}

// zeroRange fills the destination range with zero bytes
func zeroRange(fd *os.File, offset int64, length int64) error {
	punch := func(offset int64, length int64) error {
		utils.PrintLog(fmt.Sprintf("Punching %d-byte hole at offset %d", length, offset))
		flags := uint32(unix.FALLOC_FL_PUNCH_HOLE | unix.FALLOC_FL_KEEP_SIZE)
		return syscall.Fallocate(int(fd.Fd()), flags, offset, length)
	}

	err := punch(offset, length)
	if err != nil {
		return errors.Wrapf(err, "failed to punch hole at offset %d", offset)
	}

	if err != nil { // Fall back to regular pwrite
		utils.PrintLog(fmt.Sprintf("Unable to zero range %d - %d on destination, falling back to pwrite: %v", offset, offset+length, err))
		count := int64(0)
		const blocksize = 16 << 20
		buffer := bytes.Repeat([]byte{0}, blocksize)
		for count < length {
			remaining := length - count
			if remaining < blocksize {
				buffer = bytes.Repeat([]byte{0}, int(remaining))
			}
			written, err := pwrite(fd, buffer, uint64(offset))
			if err != nil {
				utils.PrintLog(fmt.Sprintf("Unable to write %d zeroes at offset %d: %v", length, offset, err))
				break
			}
			count += int64(written)
		}
	}

	return nil
}

func copyRange(fd *os.File, handle *libnbd.Libnbd, block *BlockStatusData) error {
	if (block.Flags & (libnbd.STATE_ZERO | libnbd.STATE_HOLE)) != 0 {
		err := zeroRange(fd, block.Offset, block.Length)
		if err != nil {
			return fmt.Errorf("failed to zero range at offset %d: %v", block.Offset, err)
		}
	}

	buffer := bytes.Repeat([]byte{0}, MaxPreadLength)
	count := int64(0)
	for count < block.Length {
		if block.Length-count < int64(MaxPreadLength) {
			buffer = bytes.Repeat([]byte{0}, int(block.Length-count))
		}
		length := len(buffer)

		offset := block.Offset + count
		err := handle.Pread(buffer, uint64(offset), nil)
		if err != nil {
			return fmt.Errorf("error reading from source at offset %d: %v", offset, err)
		}

		_, err = pwrite(fd, buffer, uint64(offset))
		if err != nil {
			return fmt.Errorf("failed to write data block at offset %d to local file: %v", block.Offset, err)
		}
		count += int64(length)
	}
	return nil
}

func (nbdserver *NBDServer) CopyChangedBlocks(ctx context.Context, changedAreas types.DiskChangeInfo, path string) error {
	// Copy the changed blocks from source to destination
	handle, err := libnbd.Create()
	if err != nil {
		return fmt.Errorf("failed to create libnbd handle: %v", err)
	}
	err = handle.AddMetaContext("base:allocation")
	if err != nil {
		return fmt.Errorf("failed to add meta context: %v", err)
	}
	err = handle.ConnectUri(generateSockUrl(nbdserver.tmp_dir))
	if err != nil {
		return fmt.Errorf("failed to connect to source: %v", err)
	}
	defer handle.Close()

	fd, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}

	defer fd.Close()

	totalsize := int64(0)
	for _, extent := range changedAreas.ChangedArea {
		totalsize += extent.Length
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 16)
	incrementalcopyprogress := make(chan int64)

	// Goroutine for updating progress
	go func() {
		copiedsize := int64(0)
		for progress := range incrementalcopyprogress {
			copiedsize += progress
			prog := fmt.Sprintf("Progress: %.2f%%", float64(copiedsize)/float64(totalsize)*100.0)
			utils.PrintLog(prog)
			nbdserver.progresschan <- prog
		}
	}()

	for _, extent := range changedAreas.ChangedArea {
		wg.Add(1)
		go func(extent types.DiskChangeExtent) {
			blocks := getBlockStatus(handle, extent)
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			for _, block := range blocks {
				if err := copyRange(fd, handle, block); err != nil {
					utils.PrintLog(fmt.Sprintf("Failed to copy block: %v", err))
				}
			}
			// check if context is cancelled
			select {
			case <-ctx.Done():
				if _, ok := <-incrementalcopyprogress; ok {
					close(incrementalcopyprogress)
				}
				return
			case incrementalcopyprogress <- extent.Length:
				wg.Done()
			}
		}(extent)
	}
	wg.Wait()
	close(incrementalcopyprogress)
	return nil
}

func generateSockUrl(tmp_dir string) string {
	return fmt.Sprintf("nbd+unix:///?socket=%s/nbdkit.sock", tmp_dir)
}
