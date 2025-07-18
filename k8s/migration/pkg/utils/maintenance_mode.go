package utils

import (
	"context"
	"fmt"

	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/k8s/migration/pkg/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

// CanEnterMaintenanceMode checks if an ESXi host can successfully enter maintenance mode
// with all VMs automatically migrating off to other hosts. It checks host, VM, and cluster
// settings that could block automatic VM migration.
func CanEnterMaintenanceMode(ctx context.Context, scope *scope.RollingMigrationPlanScope, vmwcreds *vjailbreakv1alpha1.VMwareCreds, hostName string, config RollingMigartionValidationConfig) (bool, string, error) {
	// List of VMs that cannot migrate
	blockedVMs := make([]string, 0)
	k8sClient := scope.Client
	// Connect to vCenter
	c, err := ValidateVMwareCreds(ctx, k8sClient, vmwcreds)
	if err != nil {
		return false, fmt.Sprintf("failed to validate vCenter connection: %v", err), fmt.Errorf("failed to validate vCenter connection: %w", err)
	}

	// Create a finder to locate objects
	finder := find.NewFinder(c, true)

	// Use default datacenter
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return false, fmt.Sprintf("error getting datacenter: %v", err), fmt.Errorf("error getting datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	// Get host system
	host, err := finder.HostSystem(ctx, hostName)
	if err != nil {
		return false, fmt.Sprintf("host not found: %v", err), fmt.Errorf("host not found: %w", err)
	}

	// Get host properties
	var hostProps mo.HostSystem
	err = host.Properties(ctx, host.Reference(), []string{"vm", "parent"}, &hostProps)
	if err != nil {
		return false, fmt.Sprintf("error getting host properties: %v", err), fmt.Errorf("error getting host properties: %w", err)
	}

	// Check if host already in maintenance mode
	var hostSummary mo.HostSystem
	err = host.Properties(ctx, host.Reference(), []string{"runtime"}, &hostSummary)
	if err != nil {
		return false, fmt.Sprintf("error getting host runtime: %v", err), fmt.Errorf("error getting host runtime: %w", err)
	}

	if hostSummary.Runtime.InMaintenanceMode {
		return true, fmt.Sprintf("Host %s already in maintenance mode", hostName), nil
	}

	// Check cluster settings
	clusterMoRef := hostProps.Parent
	var cluster mo.ClusterComputeResource
	pc := property.DefaultCollector(c)
	err = pc.RetrieveOne(ctx, *clusterMoRef, []string{"configuration", "name"}, &cluster)
	if err != nil {
		return false, fmt.Sprintf("failed to get cluster information: %v", err), fmt.Errorf("failed to get cluster information: %w", err)
	}

	if config.CheckDRSEnabled || config.CheckDRSIsFullyAutomated {
		// Check if DRS is enabled and in fully automated mode
		if cluster.Configuration.DrsConfig.Enabled == nil {
			return false, "cluster configuration not available", nil
		}

		drsEnabled := cluster.Configuration.DrsConfig.Enabled != nil && *cluster.Configuration.DrsConfig.Enabled
		drsAutomationLevel := cluster.Configuration.DrsConfig.DefaultVmBehavior

		// Check if DRS automation level supports automatic migration
		if !drsEnabled {
			return false, fmt.Sprintf("DRS not enabled on cluster %s, VM migration will not be automatic", cluster.Name), nil
		}

		if config.CheckDRSIsFullyAutomated {
			fullyAutomated := drsAutomationLevel == types.DrsBehaviorFullyAutomated
			if !fullyAutomated {
				return false, fmt.Sprintf("DRS not in fully automated mode on cluster %s, some VMs may not migrate automatically", cluster.Name), nil
			}
		}
	}

	// Check if there are any other hosts in the cluster to migrate to
	var clusterHosts mo.ClusterComputeResource
	err = pc.RetrieveOne(ctx, *clusterMoRef, []string{"host"}, &clusterHosts)
	if err != nil {
		return false, fmt.Sprintf("failed to get cluster hosts: %v", err), fmt.Errorf("failed to get cluster hosts: %w", err)
	}

	if config.CheckIfThereAreMoreThanOneHostInCluster {
		if len(clusterHosts.Host) <= 1 {
			return false, fmt.Sprintf("cluster %s has only one host, nowhere to migrate VMs", cluster.Name), nil
		}
	}

	// Check cluster resource capacity
	var clusterResourceUsageSummary mo.ClusterComputeResource
	err = pc.RetrieveOne(ctx, *clusterMoRef, []string{"summary"}, &clusterResourceUsageSummary)
	if err != nil {
		return false, fmt.Sprintf("failed to get cluster resource summary: %v", err), fmt.Errorf("failed to get cluster resource summary: %w", err)
	}

	if config.CheckClusterRemainingHostCapacity {
		// Check if remaining hosts have enough capacity
		if clusterResourceUsageSummary.Summary != nil && clusterResourceUsageSummary.Summary.GetComputeResourceSummary() != nil {
			summary := clusterResourceUsageSummary.Summary.GetComputeResourceSummary()

			// Calculate if removing this host would leave enough capacity
			// This is a basic estimation - in reality vSphere DRS has more sophisticated algorithms
			effectiveHosts := len(clusterHosts.Host) - 1 // Removing the host we're checking
			if effectiveHosts > 0 {
				currentCPUUsage := float64(summary.TotalCpu-summary.EffectiveCpu) / float64(summary.TotalCpu)
				currentMemUsage := float64(summary.TotalMemory-summary.EffectiveMemory) / float64(summary.TotalMemory)

				// A rough estimation assuming equal hosts
				projectedCPUUsage := currentCPUUsage * float64(len(clusterHosts.Host)) / float64(effectiveHosts)
				projectedMemUsage := currentMemUsage * float64(len(clusterHosts.Host)) / float64(effectiveHosts)

				if projectedCPUUsage > 1 || projectedMemUsage > 1 {
					return false, fmt.Sprintf("Cluster might not have enough resources after removing host. Projected usage: CPU %.2f%%, Memory %.2f%%",
						projectedCPUUsage*100, projectedMemUsage*100), nil
				}
			}
		}
	}

	// Check VMs on the host
	if len(hostProps.Vm) == 0 {
		return true, fmt.Sprintf("No VMs on host %s, maintenance mode can be entered", hostName), nil
	}

	if config.CheckVMsAreNotBlockedForMigration {
		// Get properties for all VMs on the host
		var vms []mo.VirtualMachine
		var vmRefs []types.ManagedObjectReference
		vmRefs = append(vmRefs, hostProps.Vm...)

		err = pc.Retrieve(ctx, vmRefs, []string{"name", "runtime", "config", "summary"}, &vms)
		if err != nil {
			return false, fmt.Sprintf("failed to retrieve VM properties: %v", err), fmt.Errorf("failed to retrieve VM properties: %w", err)
		}

		for _, vm := range vms {
			blockedVMs = append(blockedVMs, CheckVMForMaintenanceMode(vm))
		}

		if len(blockedVMs) > 0 {
			return false, fmt.Sprintf("some VMs on host %s are blocked for migration: %v", hostName, blockedVMs), nil
		}
	}

	return true, "", nil
}

// GetMaintenanceModeOptions creates a maintenance mode specification for the specified ESXi host
// It configures appropriate options based on the host's capabilities, including VSAN settings
func GetMaintenanceModeOptions(ctx context.Context, k8sClient client.Client, vmwcreds *vjailbreakv1alpha1.VMwareCreds, hostName string) (*types.HostMaintenanceSpec, error) {
	// Connect to vCenter
	c, err := ValidateVMwareCreds(ctx, k8sClient, vmwcreds)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vCenter connection: %w", err)
	}

	// Create a finder to locate objects
	finder := find.NewFinder(c, true)

	// Use default datacenter
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	// Get host system
	host, err := finder.HostSystem(ctx, hostName)
	if err != nil {
		return nil, fmt.Errorf("host not found: %w", err)
	}

	// Create maintenance spec with defaults
	spec := &types.HostMaintenanceSpec{
		// Default to migrating VMs to other hosts
		VsanMode: &types.VsanHostDecommissionMode{
			ObjectAction: string(types.VsanHostDecommissionModeObjectActionEnsureObjectAccessibility),
		},
	}

	// Get host capabilities to determine VSAN options if needed
	var hostProps mo.HostSystem
	err = host.Properties(ctx, host.Reference(), []string{"capability", "config"}, &hostProps)
	if err != nil {
		return nil, fmt.Errorf("error getting host properties: %w", err)
	}

	// Set appropriate options based on host capabilities
	if hostProps.Config != nil && hostProps.Config.VsanHostConfig != nil && hostProps.Config.VsanHostConfig.Enabled != nil && *hostProps.Config.VsanHostConfig.Enabled {
		// Host has VSAN enabled, set appropriate mode
		spec.VsanMode = &types.VsanHostDecommissionMode{
			ObjectAction: string(types.VsanHostDecommissionModeObjectActionEnsureObjectAccessibility),
		}
	}

	return spec, nil
}

// CheckVMForMaintenanceMode evaluates if a VM would block maintenance mode operations
// Returns an empty string if the VM won't block maintenance mode, otherwise returns VM name with reason
func CheckVMForMaintenanceMode(vm mo.VirtualMachine) string {
	// Check VM power state
	if vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOff {
		return fmt.Sprintf("%s (powered off)", vm.Name)
	}

	// Check for vMotion capability
	if vm.Runtime.Host == nil {
		return fmt.Sprintf("%s (no host information)", vm.Name)
	}

	// Check for VM specific issues that would block migration
	if vm.Config == nil {
		return fmt.Sprintf("%s (no configuration)", vm.Name)
	}

	// Check for connected devices that would block migration
	hasBlockingDevices := false
	if vm.Config.Hardware.Device != nil {
		for _, device := range vm.Config.Hardware.Device {
			// Check for connected local media like ISOs
			if cdrom, ok := device.(*types.VirtualCdrom); ok {
				if cdrom.Connectable != nil && cdrom.Connectable.Connected && cdrom.Connectable.StartConnected {
					// Check if it's a client device or ISO
					// Remote passthrough devices are client devices that would block migration
					// File-backed ISOs are typically ok for migration
					if _, ok := cdrom.Backing.(*types.VirtualCdromRemotePassthroughBackingInfo); ok {
						hasBlockingDevices = true
						break
					}

					// Also check for host device passthrough which blocks migration
					if _, ok := cdrom.Backing.(*types.VirtualCdromAtapiBackingInfo); ok {
						hasBlockingDevices = true
						break
					}
				}
			}

			// Check for USB and other passthrough devices
			if _, ok := device.(*types.VirtualUSB); ok {
				hasBlockingDevices = true
				break
			}
		}
	}

	if hasBlockingDevices {
		return fmt.Sprintf("%s (connected devices)", vm.Name)
	}

	// Check VM-Host affinity rules
	// This is a basic implementation - in a production environment you would
	// check detailed DRS rules including must/should constraints
	if vm.Config.ExtraConfig != nil {
		hasAffinityRules := false
		for _, opt := range vm.Config.ExtraConfig {
			if opt.GetOptionValue().Key == "hostAffinityRule" {
				hasAffinityRules = true
				break
			}
		}
		if hasAffinityRules {
			return fmt.Sprintf("%s (affinity rules)", vm.Name)
		}
	}

	// Check additional migration constraints

	// Check VM runtime status that could prevent migration
	if vm.Runtime.PowerState == types.VirtualMachinePowerStateSuspended {
		return fmt.Sprintf("%s (suspended state)", vm.Name)
	}

	// Check if there's a pending question on the VM (requires user input)
	if vm.Runtime.Question != nil {
		return fmt.Sprintf("%s (pending question)", vm.Name)
	}
	return ""
}

// GetValidationConfigMapForRollingMigrationPlan retrieves the validation config map for a rolling migration plan
func GetValidationConfigMapForRollingMigrationPlan(ctx context.Context, k8sClient client.Client, rollingMigrationPlan *vjailbreakv1alpha1.RollingMigrationPlan) (*corev1.ConfigMap, error) {
	var rollingMigrationPlanValidationConfig corev1.ConfigMap
	if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: getRollingMigrationPlanValidationConfigFromConfigMapName(rollingMigrationPlan.Name), Namespace: constants.NamespaceMigrationSystem}, &rollingMigrationPlanValidationConfig); err != nil {
		return nil, err
	}
	return &rollingMigrationPlanValidationConfig, nil
}

// GetRollingMigrationPlanValidationConfigFromConfigMap retrieves the validation config from a config map
func GetRollingMigrationPlanValidationConfigFromConfigMap(configMap *corev1.ConfigMap) *RollingMigartionValidationConfig {
	data := configMap.Data
	if data == nil {
		return nil
	}
	var rollingMigrationPlanValidationConfig RollingMigartionValidationConfig
	for key, value := range data {
		switch key {
		case "CheckDRSEnabled":
			rollingMigrationPlanValidationConfig.CheckDRSEnabled = value == trueString
		case "CheckDRSIsFullyAutomated":
			rollingMigrationPlanValidationConfig.CheckDRSIsFullyAutomated = value == trueString
		case "CheckIfThereAreMoreThanOneHostInCluster":
			rollingMigrationPlanValidationConfig.CheckIfThereAreMoreThanOneHostInCluster = value == trueString
		case "CheckClusterRemainingHostCapacity":
			rollingMigrationPlanValidationConfig.CheckClusterRemainingHostCapacity = value == trueString
		case "CheckVMsAreNotBlockedForMigration":
			rollingMigrationPlanValidationConfig.CheckVMsAreNotBlockedForMigration = value == trueString
		case "CheckESXiInMAAS":
			rollingMigrationPlanValidationConfig.CheckESXiInMAAS = value == trueString
		case "CheckPCDHasClusterConfigured":
			rollingMigrationPlanValidationConfig.CheckPCDHasClusterConfigured = value == trueString
		}
	}

	return &rollingMigrationPlanValidationConfig
}

// getRollingMigrationPlanValidationConfigFromConfigMapName retrieves the validation config map name for a rolling migration plan
func getRollingMigrationPlanValidationConfigFromConfigMapName(rollingMigrationPlanName string) string {
	return fmt.Sprintf("%s-validation-config", rollingMigrationPlanName)
}

// CreateDefaultValidationConfigMapForRollingMigrationPlan creates a default validation config map for a rolling migration plan
func CreateDefaultValidationConfigMapForRollingMigrationPlan(ctx context.Context, k8sClient client.Client, rollingMigrationPlan *vjailbreakv1alpha1.RollingMigrationPlan) (*corev1.ConfigMap, error) {
	data := map[string]string{
		"CheckDRSEnabled":                         trueString,
		"CheckDRSIsFullyAutomated":                trueString,
		"CheckIfThereAreMoreThanOneHostInCluster": trueString,
		"CheckClusterRemainingHostCapacity":       trueString,
		"CheckVMsAreNotBlockedForMigration":       trueString,
		"CheckESXiInMAAS":                         trueString,
		"CheckPCDHasClusterConfigured":            trueString,
	}
	rollingMigrationPlanValidationConfig := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getRollingMigrationPlanValidationConfigFromConfigMapName(rollingMigrationPlan.Name),
			Namespace: constants.NamespaceMigrationSystem,
		},
		Data: data,
	}
	if err := k8sClient.Create(ctx, &rollingMigrationPlanValidationConfig); err != nil {
		return nil, err
	}
	return &rollingMigrationPlanValidationConfig, nil
}
