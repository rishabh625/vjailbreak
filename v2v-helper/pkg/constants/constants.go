package constants

import "time"

const (
	RhelFirstBootScript = `#!/bin/bash
nmcli -t -f NAME connection show | while read -r conn; do
    nmcli con modify "$conn" ipv4.method auto ipv4.address "" ipv4.gateway ""
    nmcli con modify "$conn" ipv6.method auto ipv6.address "" ipv6.gateway ""
    nmcli con reload
    nmcli con down "$conn"
    nmcli con up "$conn"
done
systemctl enable --now serial-getty@ttyS0.service`

	MaxCPU = 9999999
	MaxRAM = 9999999

	// Number of intervals to wait for the volume to become available
	MaxIntervalCount = 60

	InspectOSCommand         = "inspect-os"
	LSBootCommand            = "ls /boot"
	XMLFileName              = "libxml.xml"
	MigrationSnapshotName    = "migration-snap"
	MaxHTTPRetryCount        = 5
	MaxVMActiveCheckCount    = 15
	VMActiveCheckInterval    = 20 * time.Second
	MigrationSystemNamespace = "migration-system"
	TrueString               = "true"

	LogsDir = "/var/log/pf9"

	EventMessageConvertingDisk                    = "Converting disk"
	EventMessageWaitingForCutOverStart            = "Waiting for VM Cutover start time"
	EventMessageCopyingChangedBlocksWithIteration = "Copying changed blocks"
	EventMessageWaitingForDataCopyStart           = "Waiting for data copy start time"
	EventMessageDataCopyStart                     = "Data copy start time reached"
	EventMessageWaitingForAdminCutOver            = "Waiting for Admin Cutover conditions to be met"
	EventMessageMigrationSucessful                = "VM created successfully"
	EventMessageMigrationFailed                   = "Trying to perform cleanup"
	EventMessageCopyingDisk                       = "Copying disk"
	EventMessageFailed                            = "Failed to"

	AcceptedHTTPStatusCodes = 202
)
