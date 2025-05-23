// Copyright © 2024 The vjailbreak authors

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/platform9/vjailbreak/v2v-helper/migrate"
	"github.com/platform9/vjailbreak/v2v-helper/nbd"
	"github.com/platform9/vjailbreak/v2v-helper/openstack"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/utils"
	"github.com/platform9/vjailbreak/v2v-helper/reporter"
	"github.com/platform9/vjailbreak/v2v-helper/vcenter"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure context is canceled when we exit

	// Initialize error reporter early
	eventReporter, err := reporter.NewReporter()
	if err != nil {
		log.Printf("Failed to create reporter: %v", err)
		return
	}

	eventReporterChan := make(chan string)
	podLabelWatcherChan := make(chan string)
	ackChan := make(chan struct{})

	defer close(eventReporterChan)
	defer close(podLabelWatcherChan)

	// Start reporter goroutines
	eventReporter.UpdatePodEvents(ctx, eventReporterChan, ackChan)
	eventReporter.WatchPodLabels(ctx, podLabelWatcherChan)

	// Helper function to report and handle errors
	handleError := func(msg string) {
		if reporter.IsRunningInPod() {
			eventReporterChan <- msg
			// Wait for the reporter to process the message
			<-ackChan
		}
		log.Print(msg)
		os.Exit(1)
	}

	client, err := utils.GetInclusterClient()
	if err != nil {
		handleError(fmt.Sprintf("Failed to get in-cluster client: %v", err))
	}

	migrationparams, err := utils.GetMigrationParams(ctx, client)
	if err != nil {
		handleError(fmt.Sprintf("Failed to get migration parameters: %v", err))
	}

	var (
		vCenterURL        = strings.TrimSpace(os.Getenv("VCENTER_HOST"))
		vCenterUserName   = strings.TrimSpace(os.Getenv("VCENTER_USERNAME"))
		vCenterPassword   = strings.TrimSpace(os.Getenv("VCENTER_PASSWORD"))
		vCenterInsecure   = strings.EqualFold(strings.TrimSpace(os.Getenv("VCENTER_INSECURE")), constants.TrueString)
		openstackInsecure = strings.EqualFold(strings.TrimSpace(os.Getenv("OS_INSECURE")), constants.TrueString)
	)

	starttime, _ := time.Parse(time.RFC3339, migrationparams.DataCopyStart)
	cutstart, _ := time.Parse(time.RFC3339, migrationparams.VMcutoverStart)
	cutend, _ := time.Parse(time.RFC3339, migrationparams.VMcutoverEnd)

	// Validate vCenter connection
	vcclient, err := vcenter.VCenterClientBuilder(ctx, vCenterUserName, vCenterPassword, vCenterURL, vCenterInsecure)
	if err != nil {
		handleError(fmt.Sprintf("Failed to validate vCenter connection: %v", err))
	}
	log.Printf("Connected to vCenter: %s\n", vCenterURL)

	// Validate OpenStack connection
	openstackclients, err := openstack.NewOpenStackClients(openstackInsecure)
	if err != nil {
		handleError(fmt.Sprintf("Failed to validate OpenStack connection: %v", err))
	}
	log.Println("Connected to OpenStack")

	// Get thumbprint
	thumbprint, err := vcenter.GetThumbprint(vCenterURL)
	if err != nil {
		handleError(fmt.Sprintf("Failed to get thumbprint: %s", err))
	}
	log.Printf("VCenter Thumbprint: %s\n", thumbprint)

	// Retrieve the source VM
	vmops, err := vm.VMOpsBuilder(ctx, *vcclient, migrationparams.SourceVMName, client)
	if err != nil {
		handleError(fmt.Sprintf("Failed to get source VM: %v", err))
	}

	migrationobj := migrate.Migrate{
		URL:              vCenterURL,
		UserName:         vCenterUserName,
		Password:         vCenterPassword,
		Insecure:         vCenterInsecure,
		Networknames:     utils.RemoveEmptyStrings(strings.Split(migrationparams.OpenstackNetworkNames, ",")),
		Networkports:     utils.RemoveEmptyStrings(strings.Split(migrationparams.OpenstackNetworkPorts, ",")),
		Volumetypes:      utils.RemoveEmptyStrings(strings.Split(migrationparams.OpenstackVolumeTypes, ",")),
		Virtiowin:        migrationparams.OpenstackVirtioWin,
		Ostype:           migrationparams.OpenstackOSType,
		Thumbprint:       thumbprint,
		Convert:          migrationparams.OpenstackConvert,
		Openstackclients: openstackclients,
		Vcclient:         vcclient,
		VMops:            vmops,
		Nbdops:           []nbd.NBDOperations{},
		EventReporter:    eventReporterChan,
		PodLabelWatcher:  podLabelWatcherChan,
		InPod:            reporter.IsRunningInPod(),
		MigrationTimes: migrate.MigrationTimes{
			DataCopyStart:  starttime,
			VMCutoverStart: cutstart,
			VMCutoverEnd:   cutend,
		},
		MigrationType:       migrationparams.MigrationType,
		PerformHealthChecks: migrationparams.PerformHealthChecks,
		HealthCheckPort:     migrationparams.HealthCheckPort,
		K8sClient:           client,
		TargetFlavorId:      migrationparams.TARGET_FLAVOR_ID,
	}

	if err := migrationobj.MigrateVM(ctx); err != nil {
		msg := fmt.Sprintf("Failed to migrate VM: %v", err)

		// Try to power on the VM if migration failed
		powerOnErr := vmops.VMPowerOn()
		if powerOnErr != nil {
			msg += fmt.Sprintf("\nAlso Failed to power on VM after migration failure: %v", powerOnErr)
		} else {
			msg += "\nVM was powered on after migration failure"
		}

		handleError(msg)
	}

	log.Println("Migration completed successfully")
	return
}
