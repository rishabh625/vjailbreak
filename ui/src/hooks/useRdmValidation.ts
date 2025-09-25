import { useMemo } from "react"
import { VmData } from "src/api/migration-templates/model"

interface UseRdmValidationProps {
  selectedVMs: Set<string>
  allVMs: VmData[]
}

export const useRdmValidation = ({
  selectedVMs,
  allVMs,
}: UseRdmValidationProps) => {
  const rdmValidation = useMemo(() => {
    if (selectedVMs.size === 0) {
      return {
        hasRdmVMs: false,
        hasValidationError: false,
        errorMessage: "",
        hasPowerStateError: false,
        powerStateErrorMessage: "",
        hasSelectionError: false,
        selectionErrorMessage: "",
        missingVMs: [],
        rdmGroups: {},
        requiredVMs: [],
      }
    }

    const selectedVmNames = Array.from(selectedVMs)
    const selectedVMsData = allVMs.filter((vm) =>
      selectedVmNames.includes(vm.name)
    )

    // Check if any selected VM has RDM disks
    const vmsWithRdm = selectedVMsData.filter((vm) => vm.hasSharedRdm)

    if (vmsWithRdm.length === 0) {
      return {
        hasRdmVMs: false,
        hasValidationError: false,
        errorMessage: "",
        hasPowerStateError: false,
        powerStateErrorMessage: "",
        hasSelectionError: false,
        selectionErrorMessage: "",
        missingVMs: [],
        rdmGroups: {},
        requiredVMs: [],
      }
    }

    // Only check power state for the selected VMs that have RDM disks
    const selectedRdmVMs = selectedVMsData.filter((vm) => vm.hasSharedRdm)

    // Check for VMs that are powered on (not powered off)
    const poweredOnRdmVMs = selectedRdmVMs.filter((vm) => {
      if (!vm.vmState) return false
      const powerState = vm.vmState.toLowerCase()
      // Consider VM as "powered on" if it's running, not if it's powered off or not running
      return (
        powerState === "running" ||
        powerState === "poweredon" ||
        powerState === "on"
      )
    })

    // Only check for power state validation errors
    const hasPoweredOnVMs = poweredOnRdmVMs.length > 0
    const hasValidationError = hasPoweredOnVMs

    // Only power state error message
    let powerStateErrorMessage = ""

    if (hasPoweredOnVMs) {
      const poweredOnVmNames = poweredOnRdmVMs.map((vm) => vm.name).join(", ")
      powerStateErrorMessage = `All VMs with shared RDM disks must be powered off for migration. Currently powered on: ${poweredOnVmNames}`
    }

    const errorMessage = powerStateErrorMessage

    return {
      hasRdmVMs: true,
      hasValidationError,
      errorMessage,
      hasPowerStateError: hasPoweredOnVMs,
      powerStateErrorMessage,
      hasSelectionError: false,
      selectionErrorMessage: "",
      missingVMs: [],
      rdmGroups: {},
      requiredVMs: [],
    }
  }, [selectedVMs, allVMs])

  return rdmValidation
}
