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
} from "@mui/material";
import { DataGrid, GridColDef, GridRow, GridRowSelectionModel } from "@mui/x-data-grid";
import { VmData } from "src/api/migration-templates/model";
import { OpenStackFlavor } from "src/api/openstack-creds/model";
import { patchVMwareMachine } from "src/api/vmware-machines/vmwareMachines";
import CustomLoadingOverlay from "src/components/grid/CustomLoadingOverlay";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import Step from "../../components/forms/Step";
import { useEffect, useState } from "react";
import * as React from "react";
import { getMigrationPlans } from "src/api/migration-plans/migrationPlans";
import { useVMwareMachinesQuery } from "src/hooks/api/useVMwareMachinesQuery";
import InfoIcon from "@mui/icons-material/Info";
import WarningIcon from "@mui/icons-material/Warning";
import WindowsIcon from "src/assets/windows_icon.svg";
import LinuxIcon from "src/assets/linux_icon.svg";

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
  const { rowSelectionModel, onAssignFlavor, ...toolbarProps } = props;

  return (
    <Box sx={{ display: 'flex', justifyContent: 'flex-end', width: '100%', padding: '4px  8px' }}>
      {rowSelectionModel.length > 0 && (
        <Button
          variant="text"
          color="primary"
          onClick={onAssignFlavor}
          size="small"
          sx={{ ml: 1 }}
        >
          Assign Flavor ({rowSelectionModel.length})
        </Button>
      )}
      <CustomSearchToolbar {...toolbarProps} />
    </Box>
  );
};

interface VmDataWithFlavor extends VmData {
  isMigrated?: boolean;
  flavorName?: string; // Add a field to store the flavor name
  flavorNotFound?: boolean; // Add a flag to indicate if a flavor wasn't found
}

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
      </Box>
    ),
  },
  {
    field: "ipAddress",
    headerName: "Current IP",
    flex: 1,
    valueGetter: (value) => value || "- ",
  },
  {
    field: "osFamily",
    headerName: "OS",
    flex: 1,
    renderCell: (params) => {
      const osFamily = params.row.osFamily || "Unknown";
      let displayValue = osFamily;
      let icon: React.ReactNode = null;

      if (osFamily.includes("windows")) {
        displayValue = "Windows";
        icon = <img src={WindowsIcon} alt="Windows" style={{ width: 20, height: 20 }} />;
      } else if (osFamily.includes("linux")) {
        displayValue = "Linux";
        icon = <img src={LinuxIcon} alt="Linux" style={{ width: 20, height: 20, }} />;
      } else {
        displayValue = "Other";
      }

      return (
        <Tooltip title={displayValue}>
          <Box sx={{ display: 'flex', alignItems: 'center', height: '100%' }}>
            {icon}
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
  // Hidden column for sorting by vmState
  {
    field: "vmState",
    headerName: "Status",
    flex: 1,
    headerClassName: 'hidden-column',
    sortable: true,
    sortComparator: (v1, v2) => {
      if (v1 === "running" && v2 === "stopped") return -1;
      if (v1 === "stopped" && v2 === "running") return 1;
      return 0;
    }
  }
];

const paginationModel = { page: 0, pageSize: 5 };

const MIGRATED_TOOLTIP_MESSAGE = "This VM is migrating or already has been migrated.";
const DISABLED_TOOLTIP_MESSAGE = "Turn on the VM to enable migration.";
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
}: VmsSelectionStepProps) {
  const [migratedVms, setMigratedVms] = useState<Set<string>>(new Set());
  const [loadingMigratedVms, setLoadingMigratedVms] = useState(false);
  const [flavorDialogOpen, setFlavorDialogOpen] = useState(false);
  const [selectedFlavor, setSelectedFlavor] = useState<string>("");
  const [rowSelectionModel, setRowSelectionModel] = useState<GridRowSelectionModel>([]);
  const [vmsWithFlavor, setVmsWithFlavor] = useState<VmDataWithFlavor[]>([]);
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");
  const [snackbarSeverity, setSnackbarSeverity] = useState<"success" | "error">("success");
  const [updating, setUpdating] = useState(false);

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
      return {
        ...vm,
        isMigrated: migratedVms.has(vm.name) || Boolean(vm.isMigrated),
        flavor,
        flavorNotFound
      };
    });
    setVmsWithFlavor(initialVmsWithFlavor);
  }, [vmList, migratedVms, openstackFlavors]);

  const handleVmSelection = (selectedRowIds: GridRowSelectionModel) => {
    setRowSelectionModel(selectedRowIds);
    const selectedVms = vmsWithFlavor.filter((vm) => selectedRowIds.includes(vm.name));
    onChange("vms")(selectedVms);
  };

  const handleOpenFlavorDialog = () => {
    if (rowSelectionModel.length === 0) return;
    setFlavorDialogOpen(true);
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
        if (rowSelectionModel.includes(vm.name)) {
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

      const selectedVmNames = rowSelectionModel as string[];

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
      onChange("vms")(updatedVms.filter((vm) => rowSelectionModel.includes(vm.name)));

      const actionText = isAutoAssign ? "cleared flavor assignment for" : "assigned flavor to";
      setSnackbarMessage(`Successfully ${actionText} ${selectedVmNames.length} VM${selectedVmNames.length > 1 ? 's' : ''}`);
      setSnackbarSeverity("success");
      setSnackbarOpen(true);

      refreshVMList();

      handleCloseFlavorDialog();
    } catch (error) {
      console.error("Error updating VM flavors:", error);
      setSnackbarMessage("Failed to assign flavor to VMs");
      setSnackbarSeverity("error");
      setSnackbarOpen(true);
    } finally {
      setUpdating(false);
    }
  };

  const handleCloseSnackbar = () => {
    setSnackbarOpen(false);
  };

  const isRowSelectable = (params) => {
    // Allow selection even if flavorNotFound, just show warning 
    // For the new API, we don't have IP address info, so we'll just use vmState
    return params.row.vmState === "running";
  };

  const getNoRowsLabel = () => {
    return "No VMs discovered";
  };

  return (
    <VmsSelectionStepContainer>
      <Step stepNumber="2" label="Select Virtual Machines to Migrate" />
      <FieldsContainer>
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
                    vmState: false  // Hide the vmState column that we use only for sorting
                  }
                }
              }}
              pageSizeOptions={[5, 10, 25]}
              localeText={{ noRowsLabel: getNoRowsLabel() }}
              rowHeight={45}
              onRowSelectionModelChange={handleVmSelection}
              rowSelectionModel={rowSelectionModel}
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
                    rowSelectionModel={rowSelectionModel}
                    onAssignFlavor={handleOpenFlavorDialog}
                  />
                ),
                loadingOverlay: () => (
                  <CustomLoadingOverlay loadingMessage="Loading VMs ..." />
                ),
                row: (props) => {
                  const isVmStopped = props.row.vmState !== "running";
                  const isMigrated = props.row.isMigrated;
                  const hasFlavorNotFound = props.row.flavorNotFound;

                  let tooltipMessage = "";
                  if (isMigrated) {
                    tooltipMessage = MIGRATED_TOOLTIP_MESSAGE;
                  } else if (isVmStopped) {
                    tooltipMessage = DISABLED_TOOLTIP_MESSAGE;
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
              disableColumnResize
              getRowClassName={(params) => {
                if (params.row.vmState !== "running" || params.row.isMigrated) {
                  return "disabled-row";
                } else {
                  return "";
                }
              }}
            />
          </Paper>
        </FormControl>
        {error && <FormHelperText error>{error}</FormHelperText>}
      </FieldsContainer>

      {/* Flavor Assignment Dialog */}
      <Dialog
        open={flavorDialogOpen}
        onClose={handleCloseFlavorDialog}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>
          Assign Flavor to {rowSelectionModel.length} {rowSelectionModel.length === 1 ? 'VM' : 'VMs'}
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
    </VmsSelectionStepContainer>
  );
}