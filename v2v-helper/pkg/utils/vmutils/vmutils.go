package vmutils

import (
	"log"

	"github.com/pkg/errors"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/xml"
	"github.com/platform9/vjailbreak/v2v-helper/vm"
)

func GenerateXMLConfig(vminfo vm.VMInfo) error {
	diskFiles := []string{}
	for _, vmdisk := range vminfo.VMDisks {
		diskFiles = append(diskFiles, vmdisk.Path)
	}
	if err := xml.GenerateXML(diskFiles, constants.XMLFileName, vminfo.Name); err != nil {
		return errors.Wrap(err, "Failed to generate XML")
	}
	log.Printf("XML file created successfully: %s", constants.XMLFileName)
	return nil
}
