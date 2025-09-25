import { getRdmDisksList } from "./rdmDisks"
import { RdmDisk } from "./model"
import { VMwareMachine } from "../vmware-machines/model"
import { VmData } from "../migration-templates/model"

/**
 * Check if a VM has shared RDM disks
 */
export const hasSharedRdmDisks = (vm: VMwareMachine): boolean => {
  return !!(vm.spec.vms.rdmDisks && vm.spec.vms.rdmDisks.length > 0)
}

/**
 * Fetch all RDM disks and build a map for quick lookup
 */
export const fetchRdmDisksMap = async (
  namespace?: string
): Promise<Map<string, RdmDisk>> => {
  try {
    const rdmDisks = await getRdmDisksList(namespace)
    const rdmDisksMap = new Map<string, RdmDisk>()

    rdmDisks.forEach((disk) => {
      rdmDisksMap.set(disk.metadata.name, disk)
    })

    return rdmDisksMap
  } catch (error) {
    console.error("Failed to fetch RDM disks:", error)
    return new Map()
  }
}

/**
 * Get RDM dependencies for a VM (other VMs that share RDM disks)
 */
export const getRdmDependencies = (
  vm: VMwareMachine,
  rdmDisksMap: Map<string, RdmDisk>
): string[] => {
  if (!vm.spec.vms.rdmDisks || vm.spec.vms.rdmDisks.length === 0) {
    return []
  }

  const dependencies = new Set<string>()

  vm.spec.vms.rdmDisks.forEach((rdmDiskName) => {
    const rdmDisk = rdmDisksMap.get(rdmDiskName)
    if (rdmDisk && rdmDisk.spec.ownerVMs) {
      rdmDisk.spec.ownerVMs.forEach((ownerVm) => {
        // Don't include the current VM in its own dependencies
        if (ownerVm !== vm.spec.vms.name) {
          dependencies.add(ownerVm)
        }
      })
    }
  })

  return Array.from(dependencies)
}

/**
 * Enhanced mapToVmData function that includes RDM information
 */
export const mapToVmDataWithRdm = (
  machines: VMwareMachine[],
  rdmDisksMap: Map<string, RdmDisk>
): VmData[] => {
  return machines.map((machine) => {
    const hasSharedRdm = hasSharedRdmDisks(machine)
    const rdmDisks = machine.spec.vms.rdmDisks || []
    const rdmDependencies = getRdmDependencies(machine, rdmDisksMap)

    return {
      id: machine.spec.vms.name,
      name: machine.spec.vms.name,
      vmState: machine.status.powerState === "running" ? "running" : "stopped",
      ipAddress: machine.spec.vms.ipAddress,
      networks: machine.spec.vms.networks || [],
      datastores: machine.spec.vms.datastores || [],
      memory: machine.spec.vms.memory,
      cpuCount: machine.spec.vms.cpu,
      isMigrated: machine.status.migrated,
      disks: machine.spec.vms.disks || [],
      targetFlavorId: machine.spec.targetFlavorId,
      labels: machine.metadata.labels,
      osFamily: machine.spec.vms.osFamily,
      esxHost:
        machine.metadata?.labels?.[`vjailbreak.k8s.pf9.io/esxi-name`] || "",
      vmWareMachineName: machine.metadata.name,
      networkInterfaces: machine.spec.vms.networkInterfaces?.map((nic) => ({
        mac: nic.mac,
        network: nic.network,
        ipAddress: nic.ipAddress,
      })),
      // RDM-related properties
      rdmDisks,
      hasSharedRdm,
      rdmDependencies,
    }
  })
}

/**
 * Get all VMs that need to be selected together due to RDM dependencies
 */
export const getRdmRequiredSelections = (
  selectedVmNames: string[],
  allVms: VmData[]
): {
  requiredVms: string[]
  missingVms: string[]
  rdmGroups: Record<string, string[]>
} => {
  const requiredVms = new Set<string>()
  const rdmGroups: Record<string, string[]> = {}

  // Add initially selected VMs
  selectedVmNames.forEach((vmName) => requiredVms.add(vmName))

  // Find all VMs that have RDM dependencies
  const vmsWithRdm = allVms.filter(
    (vm) =>
      vm.hasSharedRdm && vm.rdmDependencies && vm.rdmDependencies.length > 0
  )

  // Build RDM groups
  vmsWithRdm.forEach((vm) => {
    if (selectedVmNames.includes(vm.name)) {
      const groupKey = `rdm-group-${vm.name}`
      const groupVms = [vm.name, ...(vm.rdmDependencies || [])]
      rdmGroups[groupKey] = groupVms

      // Add all VMs in this RDM group to required selections
      groupVms.forEach((vmName) => requiredVms.add(vmName))
    }
  })

  const allRequiredVms = Array.from(requiredVms)
  const missingVms = allRequiredVms.filter(
    (vmName) => !selectedVmNames.includes(vmName)
  )

  return {
    requiredVms: allRequiredVms,
    missingVms,
    rdmGroups,
  }
}
