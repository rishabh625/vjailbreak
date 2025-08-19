// Package constants provides constant values used throughout the migration system
package constants

import (
	"time"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// TerminationPeriod defines the grace period for pod termination in seconds
const (
	TerminationPeriod = int64(120)

	// NameMaxLength defines the maximum length of a name
	K8sNameMaxLength = 63

	// HashSuffixLength defines the length of the hash suffix
	HashSuffixLength = 5

	// VMNameMaxLength defines the maximum length of a VM name excluding the hash suffix
	VMNameMaxLength = 57

	// MaxJobNameLength defines the maximum length of a job name
	MaxJobNameLength = 46 // 63 - 11 (prefix v2v-helper-) - 1 (hyphen) - 5 (hash)

	// VjailbreakNodeControllerName is the name of the vjailbreak node controller
	VjailbreakNodeControllerName = "vjailbreaknode-controller"

	// OpenstackCredsControllerName is the name of the openstack credentials controller
	OpenstackCredsControllerName = "openstackcreds-controller" //nolint:gosec // not a password string

	// VMwareCredsControllerName is the name of the vmware credentials controller
	VMwareCredsControllerName = "vmwarecreds-controller" //nolint:gosec // not a password string

	// MigrationControllerName is the name of the migration controller
	MigrationControllerName = "migration-controller"

	// RollingMigrationPlanControllerName is the name of the rolling migration plan controller
	RollingMigrationPlanControllerName = "rollingmigrationplan-controller"

	// ESXIMigrationControllerName is the name of the ESXi migration controller
	ESXIMigrationControllerName = "esximigration-controller"

	// ClusterMigrationControllerName is the name of the cluster migration controller
	ClusterMigrationControllerName = "clustermigration-controller"

	// BMConfigControllerName is the name of the BMConfig controller
	BMConfigControllerName = "bmconfig-controller"

	// K8sMasterNodeAnnotation is the annotation for k8s master node
	K8sMasterNodeAnnotation = "node-role.kubernetes.io/control-plane"

	// VMwareCredsLabel is the label for vmware credentials
	VMwareCredsLabel = "vjailbreak.k8s.pf9.io/vmwarecreds" //nolint:gosec // not a password string

	// OpenstackCredsLabel is the label for openstack credentials
	OpenstackCredsLabel = "vjailbreak.k8s.pf9.io/openstackcreds" //nolint:gosec // not a password string

	// IsPCDCredsLabel is the label for pcd credentials
	IsPCDCredsLabel = "vjailbreak.k8s.pf9.io/is-pcd" //nolint:gosec // not a password string

	// VMNameLabel is the label for vm name
	VMNameLabel = "vjailbreak.k8s.pf9.io/vm-name"

	// RollingMigrationPlanFinalizer is the finalizer for rolling migration plan
	RollingMigrationPlanFinalizer = "rollingmigrationplan.k8s.pf9.io/finalizer"

	// BMConfigFinalizer is the finalizer for BMConfig
	BMConfigFinalizer = "bmconfig.k8s.pf9.io/finalizer"

	// VMwareClusterLabel is the label for vmware cluster
	VMwareClusterLabel = "vjailbreak.k8s.pf9.io/vmware-cluster"

	// ESXiNameLabel is the label for ESXi name
	ESXiNameLabel = "vjailbreak.k8s.pf9.io/esxi-name"

	// ClusterMigrationLabel is the label for cluster migration
	ClusterMigrationLabel = "vjailbreak.k8s.pf9.io/clustermigration"

	// RollingMigrationPlanLabel is the label for rolling migration plan
	RollingMigrationPlanLabel = "vjailbreak.k8s.pf9.io/rollingmigrationplan"

	// PauseMigrationLabel is the label for pausing rolling migration plan
	PauseMigrationLabel = "vjailbreak.k8s.pf9.io/pause"

	// UserDataSecretKey is the key for user data secret
	UserDataSecretKey = "user-data"

	// CloudInitConfigKey is the key for cloud init config
	CloudInitConfigKey = "cloud-init-config"

	// RollingMigrationPlanValidationConfigKey is the key for rolling migration plan validation config
	RollingMigrationPlanValidationConfigKey = "validation-config"

	// NodeRoleMaster is the role of the master node
	NodeRoleMaster = "master"

	// InternalIPAnnotation is the annotation for internal IP
	InternalIPAnnotation = "k3s.io/internal-ip"

	// NumberOfDisksLabel is the label for number of disks
	NumberOfDisksLabel = "vjailbreak.k8s.pf9.io/disk-count"

	// OpenstackCredsFinalizer is the finalizer for openstack credentials
	OpenstackCredsFinalizer = "openstackcreds.k8s.pf9.io/finalizer" //nolint:gosec // not a password string

	// ClusterMigrationFinalizer is the finalizer for cluster migration
	ClusterMigrationFinalizer = "clustermigration.k8s.pf9.io/finalizer"

	// ESXIMigrationFinalizer is the finalizer for ESXi migration
	ESXIMigrationFinalizer = "esximigration.k8s.pf9.io/finalizer"

	// VMwareCredsFinalizer is the finalizer for vmware credentials
	VMwareCredsFinalizer = "vmwarecreds.k8s.pf9.io/finalizer" //nolint:gosec // not a password string

	// VjailbreakNodePhaseVMCreating is the phase for creating VM
	VjailbreakNodePhaseVMCreating = vjailbreakv1alpha1.VjailbreakNodePhase("CreatingVM")

	// VjailbreakNodePhaseVMCreated is the phase for VM created
	VjailbreakNodePhaseVMCreated = vjailbreakv1alpha1.VjailbreakNodePhase("VMCreated")

	// VjailbreakNodePhaseDeleting is the phase for deleting
	VjailbreakNodePhaseDeleting = vjailbreakv1alpha1.VjailbreakNodePhase("Deleting")

	// VjailbreakNodePhaseNodeReady is the phase for node ready
	VjailbreakNodePhaseNodeReady = vjailbreakv1alpha1.VjailbreakNodePhase("Ready")

	// NamespaceMigrationSystem is the namespace for migration system
	NamespaceMigrationSystem = "migration-system"

	// VjailbreakMasterNodeName is the name of the vjailbreak master node
	VjailbreakMasterNodeName = "vjailbreak-master"

	// VjailbreakNodeFinalizer is the finalizer for vjailbreak node
	VjailbreakNodeFinalizer = "vjailbreak.k8s.pf9.io/finalizer"

	// K3sTokenFileLocation is the location of the k3s token file
	K3sTokenFileLocation = "/etc/pf9/k3s/token" //nolint:gosec // not a password string

	// CredsRequeueAfter is the time to requeue after
	CredsRequeueAfter = 1 * time.Minute

	// ENVFileLocation is the location of the env file
	ENVFileLocation = "/etc/pf9/k3s.env"

	// MigrationTriggerDelay is the delay for migration trigger
	MigrationTriggerDelay = 5 * time.Second

	// MigrationReason is the reason for migration
	MigrationReason = "Migration"

	// StartCutOverYes is the value for start cut over yes
	StartCutOverYes = "yes"

	// MaxVCPUs is the maximum number of vCPUs
	OSFamilyWindows = "windows"
	OSFamilyLinux   = "linux"

	MaxVCPUs = 99999

	// MaxRAM is the maximum amount of RAM
	MaxRAM = 99999

	// StartCutOverNo is the value for start cut over no
	StartCutOverNo = "no"

	// PCDClusterNameNoCluster is the name of the PCD cluster when there is no cluster
	PCDClusterNameNoCluster = "NO CLUSTER"

	// RDMDiskControllerName is the name of the RDM disk controller
	RDMDiskControllerName = "rdmdisk-controller"
  
	// VCenterVMScanConcurrencyLimit is the limit for concurrency while scanning vCenter VMs
	VCenterVMScanConcurrencyLimit = 100

	// VMwareClusterNameStandAloneESX is the name of the VMware cluster when there is no cluster
	VMwareClusterNameStandAloneESX = "NO CLUSTER"

	// ConfigMap default values
	ChangedBlocksCopyIterationThreshold = 20

	// VMActiveWaitIntervalSeconds is the interval to wait for vm to become active
	VMActiveWaitIntervalSeconds = 20

	// VMActiveWaitRetryLimit is the number of retries to wait for vm to become active
	VMActiveWaitRetryLimit = 15

	// DefaultMigrationMethod is the default migration method
	DefaultMigrationMethod = "hot"

	// VCenterScanConcurrencyLimit is the max number of vcenter scan pods
	VCenterScanConcurrencyLimit = 100

	// VjailbreakSettingsConfigMapName is the name of the vjailbreak settings configmap
	VjailbreakSettingsConfigMapName = "vjailbreak-settings"
)

// CloudInitScript contains the cloud-init script for VM initialization
var (
	K3sCloudInitScript = `#cloud-config
password: %s
chpasswd: { expire: False }
write_files:
- path: %s
  content: |
    export IS_MASTER=%s
    export MASTER_IP=%s
    export K3S_TOKEN=%s
runcmd:
  - echo "Created k3s env variables!" > /home/ubuntu/cloud-init.log
`

	// MigrationConditionTypeDataCopy represents the condition type for data copy phase
	MigrationConditionTypeDataCopy corev1.PodConditionType = "DataCopy"

	// MigrationConditionTypeMigrating represents the condition type for migrating phase
	MigrationConditionTypeMigrating corev1.PodConditionType = "Migrating"

	// MigrationConditionTypeValidated represents the condition type for validated phase
	MigrationConditionTypeValidated corev1.PodConditionType = "Validated"
	MigrationConditionTypeFailed    corev1.PodConditionType = "Failed"

	// VMMigrationStatesEnum is a map of migration phase to state
	VMMigrationStatesEnum = map[vjailbreakv1alpha1.VMMigrationPhase]int{
		vjailbreakv1alpha1.VMMigrationPhasePending:                  0,
		vjailbreakv1alpha1.VMMigrationPhaseValidating:               1,
		vjailbreakv1alpha1.VMMigrationPhaseFailed:                   2,
		vjailbreakv1alpha1.VMMigrationPhaseAwaitingDataCopyStart:    3,
		vjailbreakv1alpha1.VMMigrationPhaseCopying:                  4,
		vjailbreakv1alpha1.VMMigrationPhaseCopyingChangedBlocks:     5,
		vjailbreakv1alpha1.VMMigrationPhaseConvertingDisk:           6,
		vjailbreakv1alpha1.VMMigrationPhaseAwaitingCutOverStartTime: 7,
		vjailbreakv1alpha1.VMMigrationPhaseAwaitingAdminCutOver:     8,
		vjailbreakv1alpha1.VMMigrationPhaseSucceeded:                9,
		vjailbreakv1alpha1.VMMigrationPhaseUnknown:                  10,
	}

	// MigrationJobTTL is the TTL for migration job
	MigrationJobTTL int32 = 300

	// PCDCloudInitScript contains the cloud-init script for PCD onboarding
	PCDCloudInitScript = `#cloud-config

# Run the cloud-init script on boot
runcmd:
  - echo "Validating prerequisites..."
  - for cmd in curl cloud-ctl; do
      if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: Required command '$cmd' is not installed. Please install it and retry." >&2
        exit 1
      fi
    done
  - echo "All prerequisites met. Proceeding with setup."
  
  - echo "Downloading and executing cloud-ctl setup script..."
  - curl -s https://cloud-ctl.s3.us-west-1.amazonaws.com/cloud-ctl-setup | bash
  - echo "Cloud-ctl setup script executed successfully."

  - echo "Configuring cloud-ctl..."
  - cloud-ctl config set \
      -u https://cloud-region1.platform9.io \
      -e admin@airctl.localnet \
      -r Region1 \
      -t service \
      -p 'xyz'
  - echo "Cloud-ctl configuration set successfully."

  - echo "Preparing the node..."
  - cloud-ctl prep-node
  - echo "Node preparation complete. Setup finished successfully."`
)
