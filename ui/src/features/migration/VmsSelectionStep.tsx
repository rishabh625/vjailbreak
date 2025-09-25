import {
  Chip,
  FormControl,
  FormHelperText,
  Paper,
  styled,
  Tooltip,
  Box,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormLabel,
  MenuItem,
  Select,
  Typography,
  Snackbar,
  Alert,
  TextField,
  CircularProgress,
  GlobalStyles,
} from "@mui/material";
import { DataGrid, GridColDef, GridRow, GridRowSelectionModel, GridToolbarColumnsButton } from "@mui/x-data-grid";
import { VmData } from "src/api/migration-templates/model";
import { OpenStackFlavor, OpenstackCreds } from "src/api/openstack-creds/model";
import { patchVMwareMachine } from "src/api/vmware-machines/vmwareMachines";
import CustomLoadingOverlay from "src/components/grid/CustomLoadingOverlay";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import Step from "../../components/forms/Step";
import { useEffect, useState, useCallback } from "react";
import { getTypographyStyles } from "../../theme/typography-utils";
import * as React from "react";
import { getMigrationPlans } from "src/api/migration-plans/migrationPlans";
import { useVMwareMachinesQuery } from "src/hooks/api/useVMwareMachinesQuery";
import InfoIcon from "@mui/icons-material/Info";
import WarningIcon from "@mui/icons-material/Warning";
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import WindowsIcon from "src/assets/windows_icon.svg";
import LinuxIcon from "src/assets/linux_icon.svg";
import { useErrorHandler } from "src/hooks/useErrorHandler";
import { validateOpenstackIPs } from "src/api/openstack-creds/openstackCreds";
import { getSecret } from "src/api/secrets/secrets";
import { VJAILBREAK_DEFAULT_NAMESPACE } from "src/api/constants";
import { useAmplitude } from "src/hooks/useAmplitude"
import { useRdmValidation } from "src/hooks/useRdmValidation"
import { RdmDiskConfigurationPanel } from "src/components/RdmDiskConfigurationPanel"
import { useRdmDisksQuery } from "src/hooks/api/useRdmDisksQuery"
import { patchRdmDisk } from "src/api/rdm-disks/rdmDisks";
import { RdmDisk } from "src/api/rdm-disks/model";

const VmsSelectionStepContainer = styled("div")(({ theme }) => ({
  display: "grid",
  gridGap: theme.spacing(1),
  "& .disabled-row": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
  "& .hidden-column": {
    display: "none"
  },
  "& .warning-row": {
    color: "#856404",
    fontWeight: "bold",
  }
}));

const FieldsContainer = styled("div")(({ theme }) => ({
  display: "grid",
  marginLeft: theme.spacing(6),
}));

// Style for Clarity icons
const CdsIconWrapper = styled('div')({
  marginRight: 8,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center'
});


const CustomToolbarWithActions = (props) => {
  const { rowSelectionModel, onAssignFlavor, onAssignRdmConfiguration, hasRdmVMs, ...toolbarProps } = props;

  return (
    <Box sx={{ display: 'flex', justifyContent: 'space-between', width: '100%', padding: '4px 8px' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <GridToolbarColumnsButton />
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        {rowSelectionModel && rowSelectionModel.length > 0 && (
          <>
            <Button
              variant="text"
              color="primary"
              onClick={onAssignFlavor}
              size="small"
            >
              Assign Flavor ({rowSelectionModel.length})
            </Button>
            {hasRdmVMs && (
              <Button
                variant="text"
                color="secondary"
                onClick={onAssignRdmConfiguration}
                size="small"
              >
                Configure RDM ({rowSelectionModel.length})
              </Button>
            )}
          </>
        )}
        <CustomSearchToolbar {...toolbarProps} />
      </Box>
    </Box>
  );
};

interface VmDataWithFlavor extends VmData {
  isMigrated?: boolean;
  flavorName?: string; // Add a field to store the flavor name
  flavorNotFound?: boolean; // Add a flag to indicate if a flavor wasn't found
  ipValidationStatus?: 'pending' | 'valid' | 'invalid' | 'validating';
  ipValidationMessage?: string;
  powerState?: string; // Add power state for IP editing logic
}

// Column definition moved inside component to access state

const paginationModel = { page: 0, pageSize: 5 };

const MIGRATED_TOOLTIP_MESSAGE = "This VM is migrating or already has been migrated.";
const FLAVOR_NOT_FOUND_MESSAGE = "Appropriate flavor not found. Please assign a flavor before selecting this VM for migration or create a flavor.";

interface VmsSelectionStepProps {
  onChange: (id: string) => (value: unknown) => void;
  error: string;
  open?: boolean;
  vmwareCredsValidated: boolean;
  openstackCredsValidated: boolean;
  sessionId?: string;
  openstackFlavors?: OpenStackFlavor[];
  vmwareCredName?: string;
  openstackCredName?: string;
  openstackCredentials?: OpenstackCreds;
}

export default function VmsSelectionStep({
  onChange,
  error,
  open = false,
  vmwareCredsValidated,
  openstackCredsValidated,
  sessionId = Date.now().toString(),
  openstackFlavors = [],
  vmwareCredName,
  openstackCredName,
  openstackCredentials,
}: VmsSelectionStepProps) {
  const { reportError } = useErrorHandler({ component: "VmsSelectionStep" });
  const { track } = useAmplitude({ component: "VmsSelectionStep" });
  const [migratedVms, setMigratedVms] = useState<Set<string>>(new Set());
  const [loadingMigratedVms, setLoadingMigratedVms] = useState(false);
  const [flavorDialogOpen, setFlavorDialogOpen] = useState(false);
  const [selectedFlavor, setSelectedFlavor] = useState<string>("");
  const [rdmConfigDialogOpen, setRdmConfigDialogOpen] = useState(false);
  const [selectedVMs, setSelectedVMs] = useState<Set<string>>(new Set());
  const [vmsWithFlavor, setVmsWithFlavor] = useState<VmDataWithFlavor[]>([]);
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");
  const [snackbarSeverity, setSnackbarSeverity] = useState<"success" | "error">("success");

  // Toast notification for IP assignments
  const [toastOpen, setToastOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState("");
  const [toastSeverityIp, setToastSeverityIp] = useState<"success" | "error" | "warning" | "info">("success");
  const [updating, setUpdating] = useState(false);

  // Toast notification helper
  const showToast = useCallback((message: string, severity: "success" | "error" | "warning" | "info" = "success") => {
    setToastMessage(message);
    setToastSeverityIp(severity);
    setToastOpen(true);
  }, []);

  const handleCloseToast = useCallback((_event?: React.SyntheticEvent | Event, reason?: string) => {
    if (reason === 'clickaway') {
      return;
    }
    setToastOpen(false);
  }, []);

  // OS assignment state
  const [vmOSAssignments, setVmOSAssignments] = useState<Record<string, string>>({});

  // RDM validation logic
  const rdmValidation = useRdmValidation({
    selectedVMs,
    allVMs: vmsWithFlavor
  });

  const { data: rdmDisks = [], isLoading: rdmDisksLoading } = useRdmDisksQuery();

  // RDM configuration state
  const [rdmConfigurations, setRdmConfigurations] = useState<Array<{
    uuid: string;
    diskName: string;
    cinderBackendPool: string;
    volumeType: string;
    source: Record<string, string>;
  }>>([]);

  // IP editing and validation state - similar to RollingMigrationForm
  const [editingIpFor, setEditingIpFor] = useState<string | null>(null);
  const [editingInterfaceIndex, setEditingInterfaceIndex] = useState<number | null>(null);
  const [tempIpValue, setTempIpValue] = useState<string>("");

  // Modal state for multi-NIC IP editing
  const [ipEditModalOpen, setIpEditModalOpen] = useState(false);
  const [editingVm, setEditingVm] = useState<VmDataWithFlavor | null>(null);
  const [modalIpValues, setModalIpValues] = useState<Record<string, string>>({});
  const [modalValidationStatus, setModalValidationStatus] = useState<Record<string, 'pending' | 'valid' | 'invalid' | 'validating'>>({});
  const [modalValidationMessages, setModalValidationMessages] = useState<Record<string, string>>({});

  // Bulk IP editing state (kept for potential future use but not accessible via UI)
  const [bulkEditDialogOpen, setBulkEditDialogOpen] = useState(false);
  const [bulkEditIPs, setBulkEditIPs] = useState<Record<string, Record<number, string>>>({});
  const [bulkValidationStatus, setBulkValidationStatus] = useState<Record<string, Record<number, 'empty' | 'valid' | 'invalid' | 'validating'>>>({});
  const [bulkValidationMessages, setBulkValidationMessages] = useState<Record<string, Record<number, string>>>({});
  const [assigningIPs, setAssigningIPs] = useState(false);

  // Define columns inside component to access state and functions
  const columns: GridColDef[] = [
    {
      field: "name",
      headerName: "VM Name",
      flex: 2.5,
      renderCell: (params) => (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Tooltip title={params.row.vmState === "running" ? "Running" : "Stopped"}>
              <CdsIconWrapper>
                {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                {/* @ts-ignore */}
                <cds-icon shape="vm" size="md" badge={params.row.vmState === "running" ? "success" : "danger"}></cds-icon>
              </CdsIconWrapper>
            </Tooltip>
            <Box>{params.value}</Box>
          </Box>
          {params.row.isMigrated && (
            <Chip
              variant="outlined"
              label="Migrated"
              color="info"
              size="small"
            />
          )}
          {params.row.flavorNotFound && (
            <Box display="flex" alignItems="center" gap={0.5}>
              <WarningIcon color="warning" fontSize="small" />
            </Box>
          )}
          {params.row.hasSharedRdm && (
            <Tooltip title="This VM has shared RDM disks">
              <Chip
                variant="outlined"
                label="RDM"
                color="secondary"
                size="small"
                sx={{ fontSize: '0.7rem', height: '20px' }}
              />
            </Tooltip>
          )}
        </Box>
      ),
    },
    {
      field: "ipAddress",
      headerName: "IP Address(es)",
      flex: 1,
      hideable: true,
      renderCell: (params) => {
        const vm = params.row as VmDataWithFlavor;
        const vmId = vm.name;
        const isSelected = selectedVMs.has(vmId);
        const powerState = vm.powerState;

        // For powered-off VMs with multiple network interfaces - Modal Design
        if (powerState === "powered-off" && vm.networkInterfaces && vm.networkInterfaces.length > 1) {
          // Compact view - single line with tooltip and Edit button
          const ipSummary = vm.networkInterfaces.map(nic => nic.ipAddress || "—").join(", ");

          const content = (
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              width: '100%',
              height: '100%',
              gap: 1
            }}>
              <Typography variant="body2" sx={{
                fontSize: '0.875rem',
                flex: 1,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis'
              }}>
                {ipSummary}
              </Typography>
              {isSelected && (
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => handleOpenIpEditModal(vm)}
                  sx={{
                    minWidth: 'auto',
                    px: 1.5,
                    py: 0.5,
                    fontSize: '0.75rem',
                    height: 28
                  }}
                >
                  Edit
                </Button>
              )}
            </Box>
          );

          return isSelected ? (
            <Tooltip title="Edit IP addresses for multiple network interfaces" arrow placement="top">
              {content}
            </Tooltip>
          ) : content;
        }

        // For single interface or when not using multi-interface modal
        const currentIp = vm.ipAddress || "—";
        const isEditing = editingIpFor === vmId && editingInterfaceIndex === null;

        // Only allow editing if VM is powered off
        if ((isSelected || isEditing) && powerState === "powered-off") {
          return (
            <Box sx={{ display: 'flex', alignItems: 'center', width: '100%', height: '100%' }}>
              <TextField
                value={isEditing ? tempIpValue : currentIp === "—" ? "" : currentIp}
                onChange={(e) => {
                  if (!isEditing) {
                    setTempIpValue(e.target.value);
                    setEditingIpFor(vmId);
                    setEditingInterfaceIndex(null);
                  } else {
                    setTempIpValue(e.target.value);
                  }
                }}
                onBlur={() => {
                  if (isEditing) {
                    handleSaveIP(vmId, vm.networkInterfaces ? 0 : undefined);
                  }
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    handleSaveIP(vmId, vm.networkInterfaces ? 0 : undefined);
                  } else if (e.key === 'Escape') {
                    handleCancelEditingIP();
                  }
                }}
                size="small"
                sx={{
                  minWidth: 100,
                  '& .MuiInputBase-root': {
                    height: '32px'
                  },
                  '& .MuiInputBase-input': {
                    padding: '4px 8px',
                    fontSize: '0.875rem'
                  }
                }}
                placeholder="Enter IP address"
              />
            </Box>
          );
        }

        if (powerState === "powered-on") {
          return (
            <Tooltip title="IP assignment is only available for powered-off VMs" arrow>
              <Box sx={{
                display: 'flex',
                alignItems: 'center',
                width: '100%',
                height: '100%',
              }}>
                <Typography variant="body2">
                  {currentIp}
                </Typography>
              </Box>
            </Tooltip>
          );
        }

        return (
          <Box sx={{
            display: 'flex',
            alignItems: 'center',
            width: '100%',
            height: '100%'
          }}>
            <Typography variant="body2">
              {currentIp}
            </Typography>
          </Box>
        );
      },
    },
    {
      field: "osFamily",
      headerName: "Operating System",
      flex: 1,
      hideable: true,
      renderCell: (params) => {
        const vmId = params.row.id;
        const isSelected = selectedVMs.has(vmId);
        const powerState = params.row?.powerState;
        const detectedOsFamily = params.row?.osFamily;
        const assignedOsFamily = vmOSAssignments[vmId];
        const currentOsFamily = assignedOsFamily || detectedOsFamily;


        // Show dropdown for ALL powered-off VMs (allows changing selection)
        if (isSelected && powerState === "powered-off") {
          return (
            <Box sx={{ display: 'flex', alignItems: 'center', height: '100%' }}>
              <Select
                size="small"
                value={(() => {
                  if (!currentOsFamily || currentOsFamily === "Unknown") return "";
                  const osLower = currentOsFamily.toLowerCase();
                  if (osLower.includes("windows")) return "windowsGuest";
                  if (osLower.includes("linux")) return "linuxGuest";
                  return "";
                })()}
                onChange={(e) => handleOSAssignment(vmId, e.target.value)}
                displayEmpty
                sx={{
                  minWidth: 120,
                  '& .MuiSelect-select': {
                    padding: '4px 8px',
                    fontSize: '0.875rem'
                  }
                }}
              >
                <MenuItem value="">
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.secondary' }}>
                    <WarningIcon sx={{ fontSize: 16 }} />
                    <em>Select OS</em>
                  </Box>
                </MenuItem>
                <MenuItem value="windowsGuest">
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <img src={WindowsIcon} alt="Windows" style={{ width: 16, height: 16 }} />
                    Windows
                  </Box>
                </MenuItem>
                <MenuItem value="linuxGuest">
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <img src={LinuxIcon} alt="Linux" style={{ width: 16, height: 16 }} />
                    Linux
                  </Box>
                </MenuItem>
              </Select>
            </Box>
          );
        }

        let displayValue = currentOsFamily || "Unknown";
        let icon: React.ReactNode = null;

        if (currentOsFamily && currentOsFamily.toLowerCase().includes("windows")) {
          displayValue = "Windows";
          icon = <img src={WindowsIcon} alt="Windows" style={{ width: 20, height: 20 }} />;
        } else if (currentOsFamily && currentOsFamily.toLowerCase().includes("linux")) {
          displayValue = "Linux";
          icon = <img src={LinuxIcon} alt="Linux" style={{ width: 20, height: 20 }} />;
        } else if (currentOsFamily && currentOsFamily !== "Unknown") {
          displayValue = "Unknown";
        }

        return (
          <Tooltip title={powerState === "powered-off" ?
            ((!currentOsFamily || currentOsFamily === "Unknown") ?
              "OS assignment required for powered-off VMs" :
              "Click to change OS selection") :
            displayValue}>
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              height: '100%',
              gap: 1
            }}>
              {icon}
              {(!currentOsFamily || currentOsFamily === "Unknown") && (
                <WarningIcon sx={{ color: 'warning.main', fontSize: 16 }} />
              )}
              <Typography variant="body2" sx={{
                color: (!currentOsFamily || currentOsFamily === "Unknown") ? 'text.secondary' : 'text.primary'
              }}>
                {displayValue}
              </Typography>
            </Box>
          </Tooltip>
        );
      },
    },
    {
      field: "networks",
      headerName: "Network Interface(s)",
      flex: 1.2,
      valueGetter: (value: string[]) => value?.join(", ") || "- ",
    },
    {
      field: "cpuCount",
      headerName: "CPU",
      flex: 0.7,
      valueGetter: (value) => value || "- ",
    },
    {
      field: "memory",
      headerName: "Memory (MB)",
      flex: 0.9,
      valueGetter: (value) => value || "- ",
    },
    {
      field: "esxHost",
      headerName: "ESX Host",
      flex: 1,
      valueGetter: (value) => value || "—",
    },
    {
      field: "flavor",
      headerName: "Flavor",
      flex: 1,
      valueGetter: (value) => value || "auto-assign",
      renderHeader: () => (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <div style={{ fontWeight: 500 }}>Flavor</div>
          <Tooltip title="Target OpenStack flavor to be assigned to this VM after migration.">
            <InfoIcon fontSize="small" sx={{ color: 'info.info', opacity: 0.7, cursor: 'help' }} />
          </Tooltip>
        </Box>
      ),
    },
    {
      field: "rdmDisks",
      headerName: "RDM Disks",
      flex: 1.2,
      hideable: true,
      valueGetter: (value: string[]) => value?.join(", ") || "—",
      renderHeader: () => (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <div style={{ fontWeight: 500 }}>RDM Disks</div>
          <Tooltip title="Raw Device Mapping disks associated with this VM.">
            <InfoIcon fontSize="small" sx={{ color: 'info.info', opacity: 0.7, cursor: 'help' }} />
          </Tooltip>
        </Box>
      ),
    },
    // Hidden column for sorting by vmState
    {
      field: "vmState",
      headerName: "Status",
      flex: 1,
      sortable: true,
      sortComparator: (v1, v2) => {
        if (v1 === "running" && v2 === "stopped") return -1;
        if (v1 === "stopped" && v2 === "running") return 1;
        return 0;
      }
    }
  ];

  // IP validation and utility functions
  const isValidIPAddress = (ip: string): boolean => {
    const ipRegex = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
    return ipRegex.test(ip);
  };

  const getOpenstackAccessInfo = async (openstackCredData: OpenstackCreds) => {
    const spec = openstackCredData.spec;

    // If there's a secretRef, fetch the credentials from the secret
    if (spec.secretRef?.name) {
      try {
        const secret = await getSecret(spec.secretRef.name, VJAILBREAK_DEFAULT_NAMESPACE);
        if (secret && secret.data) {
          return {
            authUrl: secret.data.OS_AUTH_URL || '',
            domainName: secret.data.OS_DOMAIN_NAME || 'default',
            insecure: secret.data.OS_INSECURE === 'true' || true,
            password: secret.data.OS_PASSWORD || '',
            regionName: secret.data.OS_REGION_NAME || '',
            tenantName: secret.data.OS_TENANT_NAME || '',
            username: secret.data.OS_USERNAME || '',
          };
        }
      } catch (error) {
        console.error('Failed to fetch OpenStack credentials from secret:', error);
        throw new Error('Failed to fetch OpenStack credentials from secret');
      }
    }

    return {
      authUrl: '',
      domainName: 'default',
      insecure: true,
      password: '',
      regionName: '',
      tenantName: '',
      username: '',
    };
  };

  useEffect(() => {
    if (!open) {
      setSelectedVMs(new Set());
    }
  }, [open]);

  const {
    data: vmList = [],
    isLoading: loadingVms,
    refetch: refreshVMList
  } = useVMwareMachinesQuery({
    vmwareCredsValidated,
    openstackCredsValidated,
    enabled: open,
    sessionId,
    vmwareCredName
  });

  useEffect(() => {
    const fetchMigratedVms = async () => {
      if (!open) return;

      setLoadingMigratedVms(true);
      try {
        const plans = await getMigrationPlans();
        const migratedVmSet = new Set<string>();

        plans.forEach(plan => {
          plan.spec.virtualMachines.forEach(vmList => {
            vmList.forEach(vm => migratedVmSet.add(vm));
          });
        });

        setMigratedVms(migratedVmSet);
      } catch (error) {
        console.error("Error fetching migrated VMs:", error);
      } finally {
        setLoadingMigratedVms(false);
      }
    };

    fetchMigratedVms();
  }, [open, vmList]);

  useEffect(() => {
    const initialVmsWithFlavor = vmList.map(vm => {
      let flavor = "";
      if (vm.targetFlavorId) {
        const foundFlavor = openstackFlavors.find(f => f.id === vm.targetFlavorId);
        if (foundFlavor) {
          flavor = foundFlavor.name;
        } else {
          flavor = vm.targetFlavorId;
        }
      }

      // Check for NOT_FOUND label for OpenStack credentials
      const flavorNotFound = openstackCredName ? vm.labels?.[openstackCredName] === "NOT_FOUND" : false;

      // Map power state from vmState 
      const powerState = vm.vmState === "running" ? "powered-on" : "powered-off";

      // Create comma-separated IP string from networkInterfaces
      const allIPs = vm.networkInterfaces
        ? vm.networkInterfaces
          .map(nic => nic.ipAddress)
          .filter(ip => ip && ip.trim() !== "")
          .join(", ")
        : vm.ipAddress || "";

      // Use assigned OS family if available, otherwise use the VM's detected OS family
      const assignedOsFamily = vmOSAssignments[vm.name];
      const finalOsFamily = assignedOsFamily || vm.osFamily;

      return {
        ...vm,
        ipAddress: allIPs || "—", // Update the main IP field to contain comma-separated IPs
        isMigrated: migratedVms.has(vm.name) || Boolean(vm.isMigrated),
        flavor,
        flavorNotFound,
        powerState,
        osFamily: finalOsFamily, // Use the assigned OS family or fallback to detected
        ipValidationStatus: 'pending' as const,
        ipValidationMessage: '',
      };
    });
    setVmsWithFlavor(initialVmsWithFlavor);
  }, [vmList, migratedVms, openstackFlavors, openstackCredName, vmOSAssignments]);

  // Separate effect for cleaning up selections when VM list changes
  useEffect(() => {
    if (vmsWithFlavor.length === 0) return;

    // Clean up selection - remove VMs that no longer exist
    const availableVmNames = new Set(vmsWithFlavor.map(vm => vm.name));
    const cleanedSelection = new Set(
      Array.from(selectedVMs).filter(vmName => availableVmNames.has(vmName))
    );

    if (cleanedSelection.size !== selectedVMs.size) {
      setSelectedVMs(cleanedSelection);
      // Update parent with cleaned selection
      const selectedVmData = vmsWithFlavor.filter(vm => cleanedSelection.has(vm.name));
      onChange("vms")(selectedVmData);
    }
  }, [vmsWithFlavor, selectedVMs, onChange]);

  useEffect(() => {
    if (vmsWithFlavor.length > 0 && selectedVMs.size > 0) {
      const selectedVmData = vmsWithFlavor.filter(vm => selectedVMs.has(vm.name));
      onChange("vms")(selectedVmData);

      // Also pass RDM configurations if available
      if (rdmConfigurations.length > 0) {
        onChange("rdmConfigurations")(rdmConfigurations);
      }
    }
  }, [vmsWithFlavor, selectedVMs, rdmConfigurations, onChange]);

  const handleVmSelection = (selectedRowIds: GridRowSelectionModel) => {
    // Update selection based on the difference
    const newSelection = new Set(selectedVMs);

    // Add newly selected items
    selectedRowIds.forEach(id => {
      if (!selectedVMs.has(id as string)) {
        newSelection.add(id as string);
      }
    });

    // Remove deselected items
    selectedVMs.forEach(id => {
      if (!selectedRowIds.includes(id)) {
        newSelection.delete(id);
      }
    });

    setSelectedVMs(newSelection);

    // Use persistent selection for onChange callback
    const selectedVmData = vmsWithFlavor.filter((vm) => newSelection.has(vm.name));
    onChange("vms")(selectedVmData);

    // Also pass RDM configurations if available
    if (rdmConfigurations.length > 0) {
      onChange("rdmConfigurations")(rdmConfigurations);
    }
  };

  // IP editing handler functions
  const handleCancelEditingIP = () => {
    setEditingIpFor(null);
    setEditingInterfaceIndex(null);
    setTempIpValue("");
  };

  const handleSaveIP = async (vmName: string, interfaceIndex?: number) => {
    const vm = vmsWithFlavor.find(v => v.name === vmName);
    if (!vm || !tempIpValue.trim()) {
      handleCancelEditingIP();
      return;
    }

    if (!isValidIPAddress(tempIpValue.trim())) {
      return;
    }

    try {
      if (openstackCredentials && interfaceIndex !== undefined) {
        const accessInfo = await getOpenstackAccessInfo(openstackCredentials);
        const validationResult = await validateOpenstackIPs({
          ip: [tempIpValue.trim()],
          accessInfo
        });

        const isValid = validationResult.isValid[0];
        const reason = validationResult.reason[0];
        console.error('IP validation failed:', reason);

        if (!isValid) {
          return;
        }
      }

      // Update the VM with new IP for specific interface
      if (interfaceIndex !== undefined && vm.networkInterfaces) {
        const updatedInterfaces = [...vm.networkInterfaces];
        updatedInterfaces[interfaceIndex] = {
          ...updatedInterfaces[interfaceIndex],
          ipAddress: tempIpValue.trim()
        };

        if (vm.vmWareMachineName) {
          await patchVMwareMachine(vm.vmWareMachineName, {
            spec: {
              vms: {
                networkInterfaces: updatedInterfaces
              }
            }
          });
        }

        // Update local state - recalculate comma-separated IP string
        const updatedVMs = vmsWithFlavor.map(v => {
          if (v.name === vmName) {
            const allIPs = updatedInterfaces
              .map(nic => nic.ipAddress)
              .filter(ip => ip && ip.trim() !== "")
              .join(", ");
            return {
              ...v,
              networkInterfaces: updatedInterfaces,
              ipAddress: allIPs || "—"
            };
          }
          return v;
        });
        setVmsWithFlavor(updatedVMs);
      } else {
        // Fallback for single IP assignment
        if (vm.vmWareMachineName) {
          await patchVMwareMachine(vm.vmWareMachineName, {
            spec: {
              vms: {
                assignedIp: tempIpValue.trim()
              }
            }
          });
        }

        // Update local state
        const updatedVMs = vmsWithFlavor.map(v =>
          v.name === vmName ? { ...v, ipAddress: tempIpValue.trim() } : v
        );
        setVmsWithFlavor(updatedVMs);
      }

      handleCancelEditingIP();

      // Show success toast
      showToast(`IP address successfully updated for VM "${vm?.name}"`);

    } catch (error) {
      reportError(error as Error, {
        context: 'ip-assignment',
        metadata: { vmName, interfaceIndex, action: 'ip-assignment' }
      });
      showToast(`Failed to update IP address for VM "${vm?.name}"`, "error");
    }
  };

  // Modal functions for multi-NIC IP editing
  const handleOpenIpEditModal = (vm: VmDataWithFlavor) => {
    setEditingVm(vm);
    // Initialize modal values with current IP addresses
    const ipValues: Record<string, string> = {};
    const validationStatus: Record<string, 'pending' | 'valid' | 'invalid' | 'validating'> = {};

    if (vm.networkInterfaces) {
      vm.networkInterfaces.forEach((nic, index) => {
        const key = `interface-${index}`;
        ipValues[key] = nic.ipAddress || "";
        validationStatus[key] = nic.ipAddress ? 'valid' : 'pending';
      });
    }

    setModalIpValues(ipValues);
    setModalValidationStatus(validationStatus);
    setModalValidationMessages({});
    setIpEditModalOpen(true);
  };

  const handleCloseIpEditModal = () => {
    setIpEditModalOpen(false);
    setEditingVm(null);
    setModalIpValues({});
    setModalValidationStatus({});
    setModalValidationMessages({});
  };

  const handleModalIpChange = (interfaceIndex: number, value: string) => {
    const key = `interface-${interfaceIndex}`;
    setModalIpValues(prev => ({
      ...prev,
      [key]: value
    }));

    // Reset validation state for this field
    setModalValidationStatus(prev => ({
      ...prev,
      [key]: 'pending'
    }));
    setModalValidationMessages(prev => ({
      ...prev,
      [key]: ""
    }));
  };

  const handleSaveModalIPs = async () => {
    if (!editingVm || !editingVm.networkInterfaces) {
      handleCloseIpEditModal();
      return;
    }

    try {
      // Validate all IP addresses first
      const ipsToValidate: string[] = [];
      const validIpMap: Record<number, string> = {};
      let hasValidationErrors = false;

      for (let i = 0; i < editingVm.networkInterfaces.length; i++) {
        const key = `interface-${i}`;
        const ipValue = modalIpValues[key]?.trim();

        if (ipValue && ipValue !== "—") {
          if (!isValidIPAddress(ipValue)) {
            setModalValidationMessages(prev => ({
              ...prev,
              [key]: "Invalid IP address format"
            }));
            setModalValidationStatus(prev => ({
              ...prev,
              [key]: 'invalid'
            }));
            hasValidationErrors = true;
          } else {
            validIpMap[i] = ipValue;
            ipsToValidate.push(ipValue);
            setModalValidationStatus(prev => ({
              ...prev,
              [key]: 'validating'
            }));
          }
        }
      }

      if (hasValidationErrors) {
        return;
      }

      // Validate IPs with backend if there are any to validate
      if (ipsToValidate.length > 0 && openstackCredentials) {
        try {
          const accessInfo = await getOpenstackAccessInfo(openstackCredentials);
          await validateOpenstackIPs({
            ip: ipsToValidate,
            accessInfo
          });
          // Mark all as valid if validation passes
          Object.keys(validIpMap).forEach(interfaceIndex => {
            const key = `interface-${interfaceIndex}`;
            setModalValidationStatus(prev => ({
              ...prev,
              [key]: 'valid'
            }));
          });
        } catch (error) {
          console.error("IP validation failed:", error);
          Object.keys(validIpMap).forEach(interfaceIndex => {
            const key = `interface-${interfaceIndex}`;
            setModalValidationMessages(prev => ({
              ...prev,
              [key]: "IP validation failed"
            }));
            setModalValidationStatus(prev => ({
              ...prev,
              [key]: 'invalid'
            }));
          });
          return;
        }
      }

      // Update the VM's network interfaces with the new IP values
      const updatedVMs = vmsWithFlavor.map(vmItem => {
        if (vmItem.name === editingVm.name && vmItem.networkInterfaces) {
          const updatedInterfaces = vmItem.networkInterfaces.map((nic, index) => {
            const key = `interface-${index}`;
            const newIpValue = modalIpValues[key]?.trim();

            return {
              ...nic,
              ipAddress: newIpValue || nic.ipAddress
            };
          });

          // Recalculate comma-separated IP string
          const allIPs = updatedInterfaces
            .map(nic => nic.ipAddress)
            .filter(ip => ip && ip.trim() !== "")
            .join(", ");

          return {
            ...vmItem,
            networkInterfaces: updatedInterfaces,
            ipAddress: allIPs || "—"
          };
        }
        return vmItem;
      });

      // Update backend with patch call
      if (editingVm.vmWareMachineName) {
        const updatedInterfaces = editingVm.networkInterfaces?.map((nic, index) => {
          const key = `interface-${index}`;
          const newIpValue = modalIpValues[key]?.trim();
          return {
            ...nic,
            ipAddress: newIpValue || nic.ipAddress
          };
        });

        await patchVMwareMachine(editingVm.vmWareMachineName, {
          spec: {
            vms: {
              networkInterfaces: updatedInterfaces
            }
          }
        });
      }

      setVmsWithFlavor(updatedVMs);
      handleCloseIpEditModal();

      // Show success toast
      showToast(`IP addresses successfully updated for VM "${editingVm.name}"`);

      // Track the analytics event
      track('ip_addresses_updated', {
        vm_name: editingVm.name,
        interface_count: editingVm.networkInterfaces.length,
        action: 'modal_multi_ip_update'
      });

    } catch (error) {
      reportError(error as Error, {
        context: 'modal-multi-ip-update',
        metadata: {
          vm_name: editingVm.name,
          modalIpValues: modalIpValues,
          action: 'modal-multi-ip-update'
        }
      });
      showToast(`Failed to update IP addresses for VM "${editingVm.name}"`, "error");
    }
  };

  // OS assignment handler
  const handleOSAssignment = async (vmId: string, osFamily: string) => {
    try {
      // Update local state first for immediate UI feedback
      setVmOSAssignments(prev => ({ ...prev, [vmId]: osFamily }));

      const vm = vmsWithFlavor.find(v => v.name === vmId);
      if (vm?.vmWareMachineName) {
        await patchVMwareMachine(vm.vmWareMachineName, {
          spec: {
            vms: {
              osFamily: osFamily
            }
          }
        });
      }


      // Track the analytics event
      track('os_family_assigned', {
        vm_name: vmId,
        os_family: osFamily,
        action: 'os-family-assignment'
      });

      showToast(`OS family successfully assigned for VM "${vmId}"`);

    } catch (error) {
      reportError(error as Error, {
        context: 'os-family-assignment',
        metadata: {
          vmId: vmId,
          osFamily: osFamily,
          action: 'os-family-assignment'
        }
      });
      // Revert local state on error
      setVmOSAssignments(prev => {
        const newState = { ...prev };
        delete newState[vmId];
        return newState;
      });
      showToast(`Failed to assign OS family for VM "${vmId}": ${error instanceof Error ? error.message : 'Unknown error'}`, "error");
    }
  };

  // Bulk IP editing functions (removed - not accessible via UI anymore)

  const handleCloseBulkEditDialog = () => {
    setBulkEditDialogOpen(false);
    setBulkEditIPs({});
    setBulkValidationStatus({});
    setBulkValidationMessages({});
  };

  const handleBulkIpChange = (vmName: string, interfaceIndex: number, value: string) => {
    setBulkEditIPs(prev => ({
      ...prev,
      [vmName]: { ...prev[vmName], [interfaceIndex]: value }
    }));

    if (!value.trim()) {
      setBulkValidationStatus(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: 'empty' }
      }));
      setBulkValidationMessages(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: '' }
      }));
    } else if (!isValidIPAddress(value.trim())) {
      setBulkValidationStatus(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: 'invalid' }
      }));
      setBulkValidationMessages(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: 'Invalid IP format' }
      }));
    } else {
      setBulkValidationStatus(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: 'empty' }
      }));
      setBulkValidationMessages(prev => ({
        ...prev,
        [vmName]: { ...prev[vmName], [interfaceIndex]: '' }
      }));
    }
  };

  const handleClearAllIPs = () => {
    const clearedIPs: Record<string, Record<number, string>> = {};
    const clearedStatus: Record<string, Record<number, 'empty' | 'valid' | 'invalid' | 'validating'>> = {};

    Object.keys(bulkEditIPs).forEach(vmName => {
      clearedIPs[vmName] = {};
      clearedStatus[vmName] = {};

      Object.keys(bulkEditIPs[vmName]).forEach(interfaceIndexStr => {
        const interfaceIndex = parseInt(interfaceIndexStr);
        clearedIPs[vmName][interfaceIndex] = "";
        clearedStatus[vmName][interfaceIndex] = 'empty';
      });
    });

    setBulkEditIPs(clearedIPs);
    setBulkValidationStatus(clearedStatus);
    setBulkValidationMessages({});
  };

  const handleApplyBulkIPs = async () => {
    // Collect all IPs to apply with their VM and interface info
    const ipsToApply: Array<{ vmName: string, interfaceIndex: number, ip: string }> = [];

    Object.entries(bulkEditIPs).forEach(([vmName, interfaces]) => {
      Object.entries(interfaces).forEach(([interfaceIndexStr, ip]) => {
        if (ip.trim() !== "") {
          ipsToApply.push({
            vmName,
            interfaceIndex: parseInt(interfaceIndexStr),
            ip: ip.trim()
          });
        }
      });
    });

    if (ipsToApply.length === 0) return;

    setAssigningIPs(true);

    try {
      // Batch validation before applying any changes
      if (openstackCredentials) {
        const accessInfo = await getOpenstackAccessInfo(openstackCredentials);
        const ipList = ipsToApply.map(item => item.ip);

        // Set validating status for all IPs
        setBulkValidationStatus(prev => {
          const newStatus = { ...prev };
          ipsToApply.forEach(({ vmName, interfaceIndex }) => {
            if (!newStatus[vmName]) newStatus[vmName] = {};
            newStatus[vmName][interfaceIndex] = 'validating';
          });
          return newStatus;
        });

        const validationResult = await validateOpenstackIPs({
          ip: ipList,
          accessInfo
        });

        // Process validation results
        const validIPs: Array<{ vmName: string, interfaceIndex: number, ip: string }> = [];
        let hasInvalidIPs = false;

        ipsToApply.forEach((item, index) => {
          const isValid = validationResult.isValid[index];
          const reason = validationResult.reason[index];

          if (isValid) {
            validIPs.push(item);
            setBulkValidationStatus(prev => ({
              ...prev,
              [item.vmName]: { ...prev[item.vmName], [item.interfaceIndex]: 'valid' }
            }));
            setBulkValidationMessages(prev => ({
              ...prev,
              [item.vmName]: { ...prev[item.vmName], [item.interfaceIndex]: 'Valid' }
            }));
          } else {
            hasInvalidIPs = true;
            setBulkValidationStatus(prev => ({
              ...prev,
              [item.vmName]: { ...prev[item.vmName], [item.interfaceIndex]: 'invalid' }
            }));
            setBulkValidationMessages(prev => ({
              ...prev,
              [item.vmName]: { ...prev[item.vmName], [item.interfaceIndex]: reason }
            }));
          }
        });

        // Only proceed if ALL IPs are valid
        if (hasInvalidIPs) {
          setAssigningIPs(false);
          return;
        }

        // Apply the valid IPs to VMs
        const updatePromises = validIPs.map(async ({ vmName, interfaceIndex, ip }) => {
          try {
            const vm = vmsWithFlavor.find(v => v.name === vmName);
            if (!vm) throw new Error('VM not found');

            // Update network interfaces
            if (vm.networkInterfaces && vm.networkInterfaces[interfaceIndex]) {
              const updatedInterfaces = [...vm.networkInterfaces];
              updatedInterfaces[interfaceIndex] = {
                ...updatedInterfaces[interfaceIndex],
                ipAddress: ip
              };

              if (vm.vmWareMachineName) {
                await patchVMwareMachine(vm.vmWareMachineName, {
                  spec: {
                    vms: {
                      networkInterfaces: updatedInterfaces
                    }
                  }
                });
              }
            } else {
              // Fallback for single IP assignment
              if (vm.vmWareMachineName) {
                await patchVMwareMachine(vm.vmWareMachineName, {
                  spec: {
                    vms: {
                      assignedIp: ip
                    }
                  }
                });
              }
            }

            return { success: true, vmName, interfaceIndex, ip };
          } catch (error) {
            setBulkValidationStatus(prev => ({
              ...prev,
              [vmName]: { ...prev[vmName], [interfaceIndex]: 'invalid' }
            }));
            setBulkValidationMessages(prev => ({
              ...prev,
              [vmName]: { ...prev[vmName], [interfaceIndex]: error instanceof Error ? error.message : 'Failed to apply IP' }
            }));
            return { success: false, vmName, interfaceIndex, error };
          }
        });

        const results = await Promise.all(updatePromises);

        // Check if any updates failed
        const failedUpdates = results.filter(result => !result.success);
        if (failedUpdates.length > 0) {
          setAssigningIPs(false);
          return; // Don't close modal if any updates failed
        }

        // Update local VM state
        const updatedVMs = vmsWithFlavor.map(vm => {
          const vmUpdates = validIPs.filter(item => item.vmName === vm.name);
          if (vmUpdates.length === 0) return vm;

          const updatedVM = { ...vm };

          if (vm.networkInterfaces) {
            const updatedInterfaces = [...vm.networkInterfaces];
            vmUpdates.forEach(({ interfaceIndex, ip }) => {
              if (updatedInterfaces[interfaceIndex]) {
                updatedInterfaces[interfaceIndex] = {
                  ...updatedInterfaces[interfaceIndex],
                  ipAddress: ip
                };
              }
            });
            updatedVM.networkInterfaces = updatedInterfaces;

            // Recalculate comma-separated IP string
            const allIPs = updatedInterfaces
              .map(nic => nic.ipAddress)
              .filter(ip => ip && ip.trim() !== "")
              .join(", ");
            updatedVM.ipAddress = allIPs || "—";
          } else {
            // Fallback for single IP
            const firstUpdate = vmUpdates[0];
            if (firstUpdate) {
              updatedVM.ipAddress = firstUpdate.ip;
            }
          }

          return updatedVM;
        });
        setVmsWithFlavor(updatedVMs);

        handleCloseBulkEditDialog();
      }

    } catch (error) {
      reportError(error as Error, {
        context: 'bulk-ip-validation-assignment',
        metadata: {
          bulkEditIPs: bulkEditIPs,
          action: 'bulk-ip-validation-assignment'
        }
      });
    } finally {
      setAssigningIPs(false);
    }
  };

  const handleOpenFlavorDialog = () => {
    if (selectedVMs.size === 0) return;
    setFlavorDialogOpen(true);
  };

  const handleOpenRdmConfigurationDialog = () => {
    if (selectedVMs.size === 0) return;
    setRdmConfigDialogOpen(true);
  };

  const handleCloseRdmConfigurationDialog = () => {
    setRdmConfigDialogOpen(false);
  };

  const handleCloseFlavorDialog = () => {
    setFlavorDialogOpen(false);
    setSelectedFlavor("");
  };

  const handleFlavorChange = (event) => {
    setSelectedFlavor(event.target.value);
  };

  const handleApplyFlavor = async () => {
    if (!selectedFlavor) {
      handleCloseFlavorDialog();
      return;
    }

    setUpdating(true);

    try {
      const isAutoAssign = selectedFlavor === "auto-assign";
      const selectedFlavorObj = !isAutoAssign ? openstackFlavors.find(f => f.id === selectedFlavor) : null;
      const flavorName = isAutoAssign ? "auto-assign" : (selectedFlavorObj ? selectedFlavorObj.name : selectedFlavor);

      const updatedVms = vmsWithFlavor.map(vm => {
        if (selectedVMs.has(vm.name)) {
          return {
            ...vm,
            targetFlavorId: isAutoAssign ? "" : selectedFlavor,
            flavorName,
            // If a flavor is assigned, the VM no longer has a flavor not found issue
            flavorNotFound: isAutoAssign ? vm.flavorNotFound : false
          };
        }
        return vm;
      });

      const selectedVmNames = Array.from(selectedVMs);

      const updatePromises = selectedVmNames.map(vmName => {
        const vmwareMachineName = vmList.find(vm => vm.name === vmName)?.vmWareMachineName
        const payload = {
          spec: {
            targetFlavorId: isAutoAssign ? "" : selectedFlavor
          }
        }
        if (!vmwareMachineName) {
          return
        }
        return patchVMwareMachine(vmwareMachineName, payload)
      });

      await Promise.all(updatePromises);

      setVmsWithFlavor(updatedVms);
      onChange("vms")(updatedVms.filter((vm) => selectedVMs.has(vm.name)));

      const actionText = isAutoAssign ? "cleared flavor assignment for" : "assigned flavor to";
      setSnackbarMessage(`Successfully ${actionText} ${selectedVmNames.length} VM${selectedVmNames.length > 1 ? 's' : ''}`);
      setSnackbarSeverity("success");
      setSnackbarOpen(true);

      refreshVMList();

      handleCloseFlavorDialog();
    } catch (error) {
      reportError(error as Error, {
        context: 'vm-flavors-update',
        metadata: {
          selectedVMs: Array.from(selectedVMs),
          selectedFlavor: selectedFlavor,
          isAutoAssign: selectedFlavor === "auto-assign",
          action: 'vm-flavors-bulk-update'
        }
      });
      setSnackbarMessage("Failed to assign flavor to VMs");
      setSnackbarSeverity("error");
      setSnackbarOpen(true);
    } finally {
      setUpdating(false);
    }
  };

  // RDM disk configuration functions
  const handleApplyRdmConfigurations = async () => {
    if (!rdmConfigurations || rdmConfigurations.length === 0) {
      showToast("No RDM configurations to apply", "warning");
      return;
    }

    setUpdating(true);

    try {
      track("rdm_configuration_applied", {
        rdmDisksCount: rdmConfigurations.length,
        selectedVMsCount: selectedVMs.size,
      });

      const updatePromises = rdmConfigurations.map(async (config) => {
        // Find the RDM disk by uuid
        const rdmDisk = rdmDisks.find(disk => disk.spec.uuid === config.uuid);
        if (!rdmDisk) {
          console.warn(`RDM disk not found for diskName: ${config.diskName}`);
          return;
        }

        const payload = {
          spec: {
            openstackVolumeRef: {
              cinderBackendPool: config.cinderBackendPool,
              volumeType: config.volumeType,
              openstackCreds: openstackCredName,
            }
          }
        } as Partial<RdmDisk>;

        return patchRdmDisk(rdmDisk.metadata.name, payload);
      });

      await Promise.all(updatePromises);

      showToast(`Successfully configured ${rdmConfigurations.length} RDM disk${rdmConfigurations.length > 1 ? 's' : ''}`, "success");

      // Close the dialog after successful configuration
      handleCloseRdmConfigurationDialog();

      // Refetch RDM disks to show updated configuration
      // This would typically be handled by React Query invalidation if we had proper query management

    } catch (error) {
      reportError(error as Error, {
        context: 'rdm-disk-configuration',
        metadata: {
          rdmConfigurationsCount: rdmConfigurations.length,
          action: 'apply-rdm-configurations'
        }
      });
      showToast("Failed to configure RDM disks", "error");
    } finally {
      setUpdating(false);
    }
  };

  const handleCloseSnackbar = () => {
    setSnackbarOpen(false);
  };

  const isRowSelectable = (params) => {
    // Allow selection for both running and stopped VMs for cold migration
    // Only disable if VM is already migrated
    return !params.row.isMigrated;
  };

  const getNoRowsLabel = () => {
    return "No VMs discovered";
  };

  return (
    <VmsSelectionStepContainer>
      <Step stepNumber="2" label="Select Virtual Machines to Migrate" />
      <FieldsContainer>
        {rdmValidation.hasRdmVMs && (
          <Alert severity="info" sx={{ mb: 2 }}>
            <Box>
              <Typography variant="body2" sx={{ fontWeight: 'medium', mb: 1 }}>
                RDM (Raw Device Mapping) Migration Detected
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Try and migrate VMs with shared RDM disks together as a group. You can configure Cinder backend settings for these disks using the "Configure RDM" button.
              </Typography>
            </Box>
          </Alert>
        )}
        <FormControl error={!!error} required>
          <Paper sx={{ width: "100%", height: 389 }}>
            <DataGrid
              rows={vmsWithFlavor}
              columns={columns}
              initialState={{
                pagination: { paginationModel },
                sorting: {
                  sortModel: [{ field: 'vmState', sort: 'asc' }],
                },
                columns: {
                  columnVisibilityModel: {
                    vmState: false,  // Hide the vmState column that we use only for sorting
                    rdmDisks: false  // Hide the RDM disks column by default
                  }
                }
              }}
              pageSizeOptions={[5, 10, 25]}
              localeText={{ noRowsLabel: getNoRowsLabel() }}
              rowHeight={45}
              onRowSelectionModelChange={handleVmSelection}
              rowSelectionModel={Array.from(selectedVMs).filter(vmName =>
                vmsWithFlavor.some(vm => vm.name === vmName)
              )}
              getRowId={(row) => row.name}
              isRowSelectable={isRowSelectable}
              disableRowSelectionOnClick
              slots={{
                toolbar: (props) => (
                  <CustomToolbarWithActions
                    {...props}
                    onRefresh={() => refreshVMList()}
                    disableRefresh={loadingVms || loadingMigratedVms || !vmwareCredsValidated || !openstackCredsValidated}
                    placeholder="Search by Name, Network Interface, CPU, or Memory"
                    rowSelectionModel={Array.from(selectedVMs).filter(vmName =>
                      vmsWithFlavor.some(vm => vm.name === vmName)
                    )}
                    onAssignFlavor={handleOpenFlavorDialog}
                    onAssignRdmConfiguration={handleOpenRdmConfigurationDialog}
                    hasRdmVMs={rdmValidation.hasRdmVMs}
                  />
                ),
                loadingOverlay: () => (
                  <CustomLoadingOverlay loadingMessage="Loading VMs ..." />
                ),
                row: (props) => {
                  const isMigrated = props.row.isMigrated;
                  const hasFlavorNotFound = props.row.flavorNotFound;

                  let tooltipMessage = "";
                  if (isMigrated) {
                    tooltipMessage = MIGRATED_TOOLTIP_MESSAGE;
                  } else if (hasFlavorNotFound) {
                    tooltipMessage = FLAVOR_NOT_FOUND_MESSAGE;
                  }

                  return (
                    <Tooltip
                      title={tooltipMessage}
                      followCursor
                    >
                      <span style={{ display: 'contents' }}>
                        <GridRow {...props} />
                      </span>
                    </Tooltip>
                  );
                },
              }}
              loading={loadingVms || loadingMigratedVms}
              checkboxSelection
              disableColumnMenu
              getRowClassName={(params) => {
                if (params.row.isMigrated) {
                  return "disabled-row";
                } else {
                  return "";
                }
              }}
              keepNonExistentRowsSelected
            />
          </Paper>
        </FormControl>
        {error && <FormHelperText error>{error}</FormHelperText>}
        {/* Separate RDM Error Messages */}
        {rdmValidation.hasSelectionError && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {rdmValidation.selectionErrorMessage}
          </Alert>
        )}

        {rdmValidation.hasPowerStateError && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {rdmValidation.powerStateErrorMessage}
          </Alert>
        )}

      </FieldsContainer>

      {/* Flavor Assignment Dialog */}
      <Dialog
        open={flavorDialogOpen}
        onClose={handleCloseFlavorDialog}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>
          Assign Flavor to {selectedVMs.size} {selectedVMs.size === 1 ? 'VM' : 'VMs'}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ my: 2 }}>
            <FormLabel>Select Flavor</FormLabel>
            <Select
              fullWidth
              value={selectedFlavor}
              onChange={handleFlavorChange}
              size="small"
              sx={{ mt: 1 }}
              displayEmpty
            >
              <MenuItem value="">
                <em>Select a flavor</em>
              </MenuItem>
              <MenuItem value="auto-assign">
                <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                  <Typography variant="body1">Auto Assign</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Let OpenStack automatically assign the most suitable flavor
                  </Typography>
                </Box>
              </MenuItem>
              {openstackFlavors.map((flavor) => (
                <MenuItem key={flavor.id} value={flavor.id}>
                  <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                    <Typography variant="body1">{flavor.name}</Typography>
                    <Typography variant="caption" color="text.secondary">
                      {flavor.vcpus} vCPU, {flavor.ram / 1024}GB RAM, {flavor.disk}GB Storage
                    </Typography>
                  </Box>
                </MenuItem>
              ))}
            </Select>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseFlavorDialog}>Cancel</Button>
          <Button
            onClick={handleApplyFlavor}
            variant="contained"
            color="primary"
            disabled={!selectedFlavor || updating}
          >
            {updating ? "Applying..." : "Apply to selected VMs"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* RDM Configuration Dialog */}
      <Dialog
        open={rdmConfigDialogOpen}
        onClose={handleCloseRdmConfigurationDialog}
        fullWidth
        maxWidth="md"
      >
        <DialogTitle>
          Configure RDM Disks for {selectedVMs.size} {selectedVMs.size === 1 ? 'VM' : 'VMs'}
        </DialogTitle>
        <DialogContent>
          {/* Debug info */}
          {process.env.NODE_ENV === 'development' && (
            <Box sx={{ mb: 2, p: 1, bgcolor: 'grey.100', fontSize: '0.8rem' }}>
              Debug: hasRdmVMs={String(rdmValidation.hasRdmVMs)}, rdmDisks.length={rdmDisks.length}, loading={String(rdmDisksLoading)}
            </Box>
          )}
          {rdmDisksLoading ? (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <CircularProgress size={24} sx={{ mr: 1 }} />
              <Typography color="text.secondary">
                Loading RDM disk information...
              </Typography>
            </Box>
          ) : rdmValidation.hasRdmVMs && rdmDisks.length > 0 ? (
            <RdmDiskConfigurationPanel
              rdmDisks={rdmDisks.filter(disk => {
                // Only show RDM disks that have at least one selected VM as owner
                const selectedVMsArray = Array.from(selectedVMs);
                return disk.spec.ownerVMs.some(ownerVM => selectedVMsArray.includes(ownerVM));
              })}
              openstackCreds={openstackCredentials}
              selectedVMs={Array.from(selectedVMs)}
              onConfigurationChange={setRdmConfigurations}
            />
          ) : (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <Typography color="text.secondary">
                {rdmValidation.hasRdmVMs
                  ? "No RDM disk configurations found."
                  : "No RDM disks detected for selected VMs."
                }
              </Typography>
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ p: 2 }}>
          <Button onClick={handleCloseRdmConfigurationDialog} variant="outlined">
            Close
          </Button>
          {rdmValidation.hasRdmVMs && rdmDisks.length > 0 && (
            <Button
              onClick={handleApplyRdmConfigurations}
              variant="contained"
              color="primary"
              disabled={updating || !rdmConfigurations || rdmConfigurations.length === 0 ||
                rdmConfigurations.some(config => !config.cinderBackendPool || !config.volumeType)}
              startIcon={updating ? <CircularProgress size={16} /> : undefined}
            >
              {updating ? 'Applying Configuration...' : 'Apply RDM Configuration'}
            </Button>
          )}
        </DialogActions>
      </Dialog>

      <Snackbar
        open={snackbarOpen}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Alert onClose={handleCloseSnackbar} severity={snackbarSeverity}>
          {snackbarMessage}
        </Alert>
      </Snackbar>

      {/* Bulk IP Editor Dialog */}
      <Dialog
        open={bulkEditDialogOpen}
        onClose={handleCloseBulkEditDialog}
        maxWidth="md"
      >
        <DialogTitle>
          Edit IP Addresses for {selectedVMs.size} {selectedVMs.size === 1 ? 'VM' : 'VMs'}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ my: 2 }}>
            {/* Quick Actions */}
            <Box sx={{ mb: 3, display: 'flex', justifyContent: 'flex-end' }}>
              <Button size="small" variant="outlined" onClick={handleClearAllIPs}>
                Clear All
              </Button>
            </Box>

            {/* IP Editor Fields */}
            <Box sx={{ maxHeight: 400, overflowY: 'auto' }}>
              {Object.entries(bulkEditIPs).map(([vmName, interfaces]) => {
                const vm = vmsWithFlavor.find(v => v.name === vmName);
                if (!vm) return null;

                return (
                  <Box key={vmName} sx={{ mb: 3, p: 2, border: '1px solid #e0e0e0', borderRadius: 1 }}>
                    <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
                      {vm.name}
                    </Typography>

                    {Object.entries(interfaces).map(([interfaceIndexStr, ip]) => {
                      const interfaceIndex = parseInt(interfaceIndexStr);
                      const networkInterface = vm.networkInterfaces?.[interfaceIndex];
                      const status = bulkValidationStatus[vmName]?.[interfaceIndex];
                      const message = bulkValidationMessages[vmName]?.[interfaceIndex];

                      return (
                        <Box key={interfaceIndex} sx={{ mb: 2, display: 'flex', alignItems: 'center', gap: 2 }}>
                          <Box sx={{ width: 120, flexShrink: 0 }}>
                            <Typography variant="caption" color="text.secondary">
                              {networkInterface?.network || `Interface ${interfaceIndex + 1}`}:
                            </Typography>
                            <Typography variant="caption" display="block" color="text.secondary">
                              Current: {networkInterface?.ipAddress || vm.ipAddress || "—"}
                            </Typography>
                          </Box>
                          <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', gap: 1 }}>
                            <TextField
                              value={ip}
                              onChange={(e) => handleBulkIpChange(vmName, interfaceIndex, e.target.value)}
                              placeholder="Enter IP address"
                              size="small"
                              sx={{ flex: 1 }}
                              error={status === 'invalid'}
                              helperText={message}
                            />
                            <Box sx={{ width: 24, display: 'flex' }}>
                              {status === 'validating' && <CircularProgress size={20} />}
                              {status === 'valid' && <CheckCircleIcon color="success" fontSize="small" />}
                              {status === 'invalid' && <ErrorIcon color="error" fontSize="small" />}
                            </Box>
                          </Box>
                        </Box>
                      );
                    })}
                  </Box>
                );
              })}
            </Box>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseBulkEditDialog}>Cancel</Button>
          <Button
            onClick={handleApplyBulkIPs}
            variant="contained"
            color="primary"
            disabled={Object.values(bulkEditIPs).every(interfaces => Object.values(interfaces).every(ip => !ip.trim())) || assigningIPs}
          >
            {assigningIPs ? "Applying..." : "Apply Changes"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* IP Address Editor Modal */}
      <Dialog
        open={ipEditModalOpen}
        onClose={handleCloseIpEditModal}
        maxWidth="sm"
        PaperProps={{
          sx: {
            width: 'auto',
            maxWidth: '500px'
          }
        }}
      >
        <DialogTitle>
          <Typography variant="h6">
            Edit IP Addresses for "{editingVm?.name}"
          </Typography>
          <Typography variant="body2">
            ({editingVm?.networkInterfaces?.length || 0} Network Interfaces)
          </Typography>
        </DialogTitle>
        <DialogContent>
          <Box sx={{ mt: 2 }}>
            {editingVm?.networkInterfaces?.map((nic, index) => {
              const key = `interface-${index}`;
              const currentValue = modalIpValues[key] || "";
              const validationStatus = modalValidationStatus[key] || 'pending';
              const validationMessage = modalValidationMessages[key] || "";

              return (
                <Box key={index} sx={{ py: 2 }}>
                  <Box sx={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 3,
                    flexDirection: { xs: 'column', sm: 'row' }
                  }}>
                    <Box sx={{
                      minWidth: { sm: 200 },
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 0.5
                    }}>
                      <Typography variant="subtitle2" sx={{ fontWeight: 500 }}>
                        Network Interface {index + 1}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        Network:<strong>{nic.network}</strong>
                      </Typography>
                      {nic.mac && (
                        <Typography variant="caption" color="text.secondary">
                          MAC:<strong>{nic.mac}</strong>
                        </Typography>
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                      <TextField
                        label="IP Address"
                        value={currentValue}
                        onChange={(e) => handleModalIpChange(index, e.target.value)}
                        size="small"
                        placeholder="192.168.1.100"
                        sx={{
                          width: '200px',
                          '& .MuiInputBase-input': {
                            ...getTypographyStyles.monospace,
                          }
                        }}
                        error={validationStatus === 'invalid'}
                        helperText={validationMessage || ""}
                        InputProps={{
                          endAdornment: validationStatus === 'validating' ? (
                            <Box sx={{ display: 'flex', alignItems: 'center', mr: 1 }}>
                              <Typography variant="caption" color="text.secondary">
                                Validating...
                              </Typography>
                            </Box>
                          ) : validationStatus === 'valid' ? (
                            <Box sx={{ display: 'flex', alignItems: 'center', mr: 1 }}>
                              <Typography variant="caption" color="success.main">
                                ✓ Valid
                              </Typography>
                            </Box>
                          ) : null
                        }}
                      />
                    </Box>
                  </Box>
                </Box>
              );
            })}
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button onClick={handleCloseIpEditModal} variant="outlined">
            Cancel
          </Button>
          <Button
            onClick={handleSaveModalIPs}
            variant="contained"
            color="primary"
            disabled={(() => {
              // Check if any IPs are assigned and valid
              const hasAnyIPs = Object.values(modalIpValues).some(ip => ip && ip.trim() !== "");
              const hasInvalidIPs = Object.values(modalValidationStatus).some(status => status === 'invalid');
              const isValidating = Object.values(modalValidationStatus).some(status => status === 'validating');

              // Disable if: no IPs assigned OR any invalid IPs OR currently validating
              return !hasAnyIPs || hasInvalidIPs || isValidating;
            })()}
          >
            Save IP Addresses
          </Button>
        </DialogActions>
      </Dialog>

      {/* Add GlobalStyles similar to RollingMigrationForm */}
      <GlobalStyles
        styles={{
          '.MuiDataGrid-columnsManagement, .MuiDataGrid-columnsManagementPopover': {
            '& .MuiFormControlLabel-label': {
              fontSize: '0.875rem !important',
            },
            '& .MuiCheckbox-root': {
              padding: '4px !important',
            },
            '& .MuiListItem-root': {
              fontSize: '0.875rem !important',
              minHeight: '32px !important',
              padding: '2px 8px !important',
            },
            '& .MuiTypography-root': {
              fontSize: '0.875rem !important',
            },
            '& .MuiInputBase-input': {
              fontSize: '0.875rem !important',
            },
            '& .MuiTextField-root .MuiInputBase-input': {
              fontSize: '0.875rem !important',
            }
          }
        }}
      />

      {/* Toast Notification for IP assignments */}
      <Snackbar
        open={toastOpen}
        autoHideDuration={4000}
        onClose={handleCloseToast}
        anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <Alert
          onClose={handleCloseToast}
          severity={toastSeverityIp}
          sx={{ width: '100%' }}
          variant="standard"
        >
          {toastMessage}
        </Alert>
      </Snackbar>
    </VmsSelectionStepContainer>
  );
}