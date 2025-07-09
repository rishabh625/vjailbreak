import { Box, Typography, Drawer, styled, Paper, Tooltip, Button, Dialog, DialogTitle, DialogContent, DialogActions, Alert, Select, MenuItem, GlobalStyles, FormLabel } from "@mui/material"
import { useState, useMemo, useEffect, useCallback } from "react"
import { DataGrid, GridColDef, GridRowSelectionModel, GridToolbarColumnsButton } from "@mui/x-data-grid"
import { useNavigate } from "react-router-dom"
import Footer from "../../components/forms/Footer"
import Header from "../../components/forms/Header"
import Step from "../../components/forms/Step"
import { DrawerContent } from "src/components/forms/StyledDrawer"
import { useKeyboardSubmit } from "src/hooks/ui/useKeyboardSubmit"
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar"
import SyntaxHighlighter from 'react-syntax-highlighter/dist/esm/prism'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { getVMwareHosts, patchVMwareHost } from "src/api/vmware-hosts/vmwareHosts"
import { getVMwareMachines, patchVMwareMachine } from "src/api/vmware-machines/vmwareMachines"
import { VMwareHost } from "src/api/vmware-hosts/model"
import { VMwareMachine } from "src/api/vmware-machines/model"
import { VJAILBREAK_DEFAULT_NAMESPACE } from "src/api/constants"
import { getBMConfigList, getBMConfig } from "src/api/bmconfig/bmconfig"
import { BMConfig } from "src/api/bmconfig/model"
import MaasConfigDetailsModal from "src/pages/dashboard/BMConfigDetailsModal"
import { getOpenstackCredentials } from "src/api/openstack-creds/openstackCreds"
import { OpenstackCreds } from "src/api/openstack-creds/model"
import NetworkAndStorageMappingStep, { ResourceMap } from "./NetworkAndStorageMappingStep"
import { createRollingMigrationPlanJson, postRollingMigrationPlan, VMSequence, ClusterMapping } from "src/api/rolling-migration-plans"
import SourceDestinationClusterSelection from "./SourceDestinationClusterSelection"
// Import required APIs for creating migration resources
import { createNetworkMappingJson } from "src/api/network-mapping/helpers"
import { postNetworkMapping } from "src/api/network-mapping/networkMappings"
import { createStorageMappingJson } from "src/api/storage-mappings/helpers"
import { postStorageMapping } from "src/api/storage-mappings/storageMappings"
import { createMigrationTemplateJson } from "src/api/migration-templates/helpers"
import { postMigrationTemplate } from "src/api/migration-templates/migrationTemplates"
import useParams from "src/hooks/useParams"
import MigrationOptions from "./MigrationOptionsAlt"
import { CUTOVER_TYPES } from "./constants"
import WindowsIcon from "src/assets/windows_icon.svg";
import LinuxIcon from "src/assets/linux_icon.svg";
import WarningIcon from '@mui/icons-material/Warning';
import { useClusterData } from "./useClusterData"

import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import { TextField, CircularProgress } from "@mui/material"
import { validateOpenstackIPs } from "src/api/openstack-creds/openstackCreds"

// Import CDS icons
import "@cds/core/icon/register.js"
import { ClarityIcons, buildingIcon, clusterIcon, hostIcon, vmIcon } from "@cds/core/icon"
import { getSecret } from "src/api/secrets/secrets"

// Define types for MigrationOptions
interface FormValues extends Record<string, unknown> {
    dataCopyMethod?: string;
    dataCopyStartTime?: string;
    cutoverOption?: string;
    cutoverStartTime?: string;
    cutoverEndTime?: string;
    postMigrationScript?: string;
    retryOnFailure?: boolean;
    osFamily?: string;
}

export interface SelectedMigrationOptionsType extends Record<string, unknown> {
    dataCopyMethod: boolean;
    dataCopyStartTime: boolean;
    cutoverOption: boolean;
    cutoverStartTime: boolean;
    cutoverEndTime: boolean;
    postMigrationScript: boolean;
    osFamily: boolean;
}

// Default state for checkboxes
const defaultMigrationOptions = {
    dataCopyMethod: false,
    dataCopyStartTime: false,
    cutoverOption: false,
    cutoverStartTime: false,
    cutoverEndTime: false,
    postMigrationScript: false,
    osFamily: false,
}

type FieldErrors = { [formId: string]: string };

// Register clarity icons
ClarityIcons.addIcons(buildingIcon, clusterIcon, hostIcon, vmIcon)

// Style for Clarity icons
const CdsIconWrapper = styled('div')({
    marginRight: 8,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center'
});

const StyledDrawer = styled(Drawer)(() => ({
    "& .MuiDrawer-paper": {
        display: "grid",
        gridTemplateRows: "max-content 1fr max-content",
        width: "1400px",
        maxWidth: "90vw",
    },
    "& .hidden-column": {
        display: "none"
    },
}))

interface ESXHost {
    id: string;
    name: string;
    ip: string;
    bmcIp: string;
    maasState: string;
    vms: number;
    state: string;
    pcdHostConfigName?: string;
}

interface VM {
    id: string;
    name: string;
    ip: string;
    esxHost: string;
    networks?: string[];
    datastores?: string[];
    cpu?: number;
    memory?: number;
    powerState: string;
    osFamily?: string;
    flavor?: string;
    targetFlavorId?: string;
    ipValidationStatus?: 'pending' | 'valid' | 'invalid' | 'validating';
    ipValidationMessage?: string;
}

const esxColumns: GridColDef[] = [
    {
        field: "name",
        headerName: "ESX Name",
        flex: 1.5,
        renderCell: (params) => (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <CdsIconWrapper>
                    {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                    {/* @ts-ignore */}
                    <cds-icon shape="host" size="md" badge="info"></cds-icon>
                </CdsIconWrapper>
                {params.value}
            </Box>
        ),
    },
    {
        field: "pcdHostConfigName",
        headerName: "Host Config",
        flex: 1,
        align: "center",
        valueGetter: (value) => value || "—",
        renderCell: (params) => (
            <Box sx={{ display: 'flex', alignItems: 'center', height: '100%' }}>
                <Typography variant="body2">
                    {params.value || "—"}
                </Typography>
            </Box>
        ),
    },
    // {
    //     field: "ip",
    //     headerName: "Current IP",
    //     flex: 1,
    //     valueGetter: (value) => value || "—",
    // },
    // {
    //     field: "bmcIp",
    //     headerName: "BMC IP Address",
    //     flex: 1,
    //     valueGetter: (value) => value || "—",
    // },
    // {
    //     field: "maasState",
    //     headerName: "MaaS State",
    //     flex: 0.5,
    //     valueGetter: (value) => value || "—",
    // },
    // {
    //     field: "vms",
    //     headerName: "# VMs",
    //     flex: 0.5,
    //     valueGetter: (value) => value || "—",
    // },
    // {
    //     field: "state",
    //     headerName: "State",
    //     flex: 0.5,
    //     renderHeader: () => (
    //         <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
    //             <div style={{ fontWeight: 500 }}>State</div>
    //         </Box>
    //     ),
    // },
];

const CustomToolbarWithActions = (props) => {
    const { rowSelectionModel, onEditIPs, onAssignFlavor, ...toolbarProps } = props;

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
                            onClick={onEditIPs}
                            size="small"
                        >
                            Assign/Edit IPs ({rowSelectionModel.length})
                        </Button>
                        <Button
                            variant="text"
                            color="primary"
                            onClick={onAssignFlavor}
                            size="small"
                        >
                            Assign Flavor ({rowSelectionModel.length})
                        </Button>
                    </>
                )}
                <CustomSearchToolbar {...toolbarProps} />
            </Box>
        </Box>
    );
};

const CustomESXToolbarWithActions = (props) => {
    const { rowSelectionModel, onAddPcdHost, ...toolbarProps } = props;

    return (
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', width: '100%', padding: '4px 8px' }}>
            {rowSelectionModel && rowSelectionModel.length > 0 && (
                <Button
                    variant="text"
                    color="primary"
                    onClick={onAddPcdHost}
                    size="small"
                    sx={{ ml: 1 }}
                >
                    Add Host Config ({rowSelectionModel.length})
                </Button>
            )}
            <CustomSearchToolbar {...toolbarProps} />
        </Box>
    );
};

const MaasConfigDialog = styled(Dialog)({
    "& .MuiDialog-paper": {
        maxWidth: "900px",
        width: "100%"
    }
});

const ConfigSection = styled(Box)(({ theme }) => ({
    marginBottom: theme.spacing(3),
}));

const ConfigField = styled(Box)(({ theme }) => ({
    display: "flex",
    gap: theme.spacing(1),
    marginBottom: theme.spacing(1.5),
}));

const FieldLabel = styled(Typography)(({ theme }) => ({
    fontWeight: 500,
    minWidth: "140px",
    color: theme.palette.text.secondary,
}));

const FieldValue = styled(Typography)(({ theme }) => ({
    fontWeight: 400,
    color: theme.palette.text.primary,
}));

const CodeEditorContainer = styled(Box)(({ theme }) => ({
    border: `1px solid ${theme.palette.grey[300]}`,
    borderRadius: theme.shape.borderRadius,
    overflow: 'auto',
    position: 'relative',
    resize: 'vertical',
    minHeight: '250px',
    maxHeight: '400px',
    backgroundColor: theme.palette.common.white,
    '& pre': {
        margin: 0,
        borderRadius: 0,
        height: '100%',
        overflow: 'auto',
        fontSize: '14px',
    },
    '&::-webkit-scrollbar': {
        width: '8px',
        height: '8px',
    },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: theme.palette.grey[300],
        borderRadius: '4px',
    },
    '&::-webkit-scrollbar-track': {
        backgroundColor: theme.palette.grey[100],
    }
}));


interface RollingMigrationFormDrawerProps {
    open: boolean;
    onClose: () => void;
}

export default function RollingMigrationFormDrawer({
    open,
    onClose,
}: RollingMigrationFormDrawerProps) {
    const navigate = useNavigate();
    const [sourceCluster, setSourceCluster] = useState("");
    const [destinationPCD, setDestinationPCD] = useState("");
    const [submitting, setSubmitting] = useState(false);

    const [selectedVMwareCredName, setSelectedVMwareCredName] = useState("");

    const [selectedPcdCredName, setSelectedPcdCredName] = useState("");

    const [selectedVMs, setSelectedVMs] = useState<GridRowSelectionModel>([]);
    const [selectedESXHosts, setSelectedESXHosts] = useState<GridRowSelectionModel>([]);
    const [esxHostToPcdMapping, setEsxHostToPcdMapping] = useState<Record<string, string>>({});
    const [pcdHostConfigDialogOpen, setPcdHostConfigDialogOpen] = useState(false);
    const [selectedPcdHostConfig, setSelectedPcdHostConfig] = useState("");
    const [updatingPcdMapping, setUpdatingPcdMapping] = useState(false);

    const [loadingHosts, setLoadingHosts] = useState(false);
    const [loadingVMs, setLoadingVMs] = useState(false);

    const [orderedESXHosts, setOrderedESXHosts] = useState<ESXHost[]>([]);
    const [vmsWithAssignments, setVmsWithAssignments] = useState<VM[]>([]);

    const [maasConfigDialogOpen, setMaasConfigDialogOpen] = useState(false);
    const [maasConfigs, setMaasConfigs] = useState<BMConfig[]>([]);
    const [selectedMaasConfig, setSelectedMaasConfig] = useState<BMConfig | null>(null);
    const [loadingMaasConfig, setLoadingMaasConfig] = useState(false);
    const [maasDetailsModalOpen, setMaasDetailsModalOpen] = useState(false);

    const [networkMappings, setNetworkMappings] = useState<ResourceMap[]>([]);
    const [storageMappings, setStorageMappings] = useState<ResourceMap[]>([]);
    const [networkMappingError, setNetworkMappingError] = useState<string>("");
    const [storageMappingError, setStorageMappingError] = useState<string>("");

    const [openstackCredData, setOpenstackCredData] = useState<OpenstackCreds | null>(null);
    const [loadingOpenstackDetails, setLoadingOpenstackDetails] = useState(false);

    // IP editing and validation state
    const [editingIpFor, setEditingIpFor] = useState<string | null>(null);
    const [tempIpValue, setTempIpValue] = useState<string>("");
    const [ipValidationStatus, setIpValidationStatus] = useState<Record<string, 'pending' | 'valid' | 'invalid' | 'validating'>>({});
    const [ipValidationMessages, setIpValidationMessages] = useState<Record<string, string>>({});

    // OS assignment state
    const [vmOSAssignments, setVmOSAssignments] = useState<Record<string, string>>({});
    const [osValidationError, setOsValidationError] = useState<string>("");

    // Migration Options state
    const { params, getParamsUpdater } = useParams<FormValues>({});
    const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
    const getFieldErrorsUpdater = useCallback((key: string | number) => (value: string) => {
        setFieldErrors(prev => ({ ...prev, [key]: value }));
    }, []);
    const { params: selectedMigrationOptions, getParamsUpdater: updateSelectedMigrationOptions } =
        useParams<SelectedMigrationOptionsType>(defaultMigrationOptions);

    const { sourceData, pcdData, loadingVMware: loading, loadingPCD } = useClusterData();
    const [assigningIPs, setAssigningIPs] = useState(false);

    // IP validation error state
    const [vmIpValidationError, setVmIpValidationError] = useState<string>("");

    // ESX host config validation error state
    const [esxHostConfigValidationError, setEsxHostConfigValidationError] = useState<string>("");

    // Bulk IP editing state
    const [bulkEditDialogOpen, setBulkEditDialogOpen] = useState(false);
    const [bulkEditIPs, setBulkEditIPs] = useState<Record<string, string>>({});
    const [bulkValidationStatus, setBulkValidationStatus] = useState<Record<string, 'empty' | 'valid' | 'invalid' | 'validating'>>({});
    const [bulkValidationMessages, setBulkValidationMessages] = useState<Record<string, string>>({});

    // Flavor assignment state
    const [flavorDialogOpen, setFlavorDialogOpen] = useState(false);
    const [selectedFlavor, setSelectedFlavor] = useState("");
    const [updating, setUpdating] = useState(false);

    const paginationModel = { page: 0, pageSize: 5 };

    useEffect(() => {
        if (open) {
            fetchMaasConfigs();
        }
    }, [open]);

    const fetchMaasConfigs = async () => {
        try {
            setLoadingMaasConfig(true);
            const configs = await getBMConfigList(VJAILBREAK_DEFAULT_NAMESPACE);
            if (configs && configs.length > 0) {
                setMaasConfigs(configs);
                try {
                    const config = await getBMConfig(configs[0].metadata.name, VJAILBREAK_DEFAULT_NAMESPACE);
                    setSelectedMaasConfig(config);
                } catch (error) {
                    console.error(`Failed to fetch MAAS config:`, error);
                }
            }
        } catch (error) {
            console.error("Failed to fetch MAAS configs:", error);
        } finally {
            setLoadingMaasConfig(false);
        }
    };

    useEffect(() => {
        if (sourceCluster) {
            fetchClusterHosts();
            fetchClusterVMs();
        }
    }, [sourceCluster]);

    const fetchClusterHosts = async () => {
        if (!sourceCluster) return;

        setLoadingHosts(true);
        try {
            const parts = sourceCluster.split(":");
            const credName = parts[0];

            const sourceItem = sourceData.find(item => item.credName === credName);
            const clusterObj = sourceItem?.clusters.find(cluster =>
                cluster.id === sourceCluster
            );
            const clusterName = clusterObj?.name;

            if (!clusterName) {
                setOrderedESXHosts([]);
                setLoadingHosts(false);
                return;
            }

            const hostsResponse = await getVMwareHosts(
                VJAILBREAK_DEFAULT_NAMESPACE,
                // credName,
                "",
                clusterName
            );

            const mappedHosts: ESXHost[] = hostsResponse.items.map((host: VMwareHost) => ({
                id: host.metadata.name,
                name: host.spec.name,
                ip: "",
                bmcIp: "",
                maasState: "Unknown",
                vms: 0,
                state: "Active"
            }));

            setOrderedESXHosts(mappedHosts);
        } catch (error) {
            console.error("Failed to fetch cluster hosts:", error);
        } finally {
            setLoadingHosts(false);
        }
    };

    const fetchClusterVMs = async () => {
        if (!sourceCluster) return;

        setLoadingVMs(true);
        try {
            const parts = sourceCluster.split(":");
            const credName = parts[0];

            const sourceItem = sourceData.find(item => item.credName === credName);
            const clusterObj = sourceItem?.clusters.find(cluster =>
                cluster.id === sourceCluster
            );
            const clusterName = clusterObj?.name;

            if (!clusterName) {
                setVmsWithAssignments([]);
                setLoadingVMs(false);
                return;
            }

            const vmsResponse = await getVMwareMachines(
                VJAILBREAK_DEFAULT_NAMESPACE,
                credName
            );

            const filteredVMs = vmsResponse.items.filter((vm: VMwareMachine) => {
                const clusterLabel = vm.metadata?.labels?.[`vjailbreak.k8s.pf9.io/vmware-cluster`];
                return clusterLabel === clusterName;
            });

            const mappedVMs: VM[] = filteredVMs.map((vm: VMwareMachine) => {
                const esxiHost = vm.metadata?.labels?.[`vjailbreak.k8s.pf9.io/esxi-name`] || "";

                // Get flavor information from the VM spec  
                const targetFlavorId = vm.spec.targetFlavorId || "";
                // We'll resolve flavor names later when openstackFlavors is available
                const flavorName = targetFlavorId || "auto-assign";

                return {
                    id: vm.metadata.name,
                    name: vm.spec.vms.name || vm.metadata.name,
                    ip: vm.spec.vms.ipAddress || vm.spec.vms.assignedIp || "—",
                    esxHost: esxiHost,
                    networks: vm.spec.vms.networks,
                    datastores: vm.spec.vms.datastores,
                    cpu: vm.spec.vms.cpu,
                    memory: vm.spec.vms.memory,
                    osFamily: vm.spec.vms.osFamily,
                    flavor: flavorName,
                    targetFlavorId: targetFlavorId,
                    powerState: vm.status.powerState === "running" ? "powered-on" : "powered-off",
                    ipValidationStatus: 'pending',
                    ipValidationMessage: ''
                };
            });

            setVmsWithAssignments(mappedVMs);
        } catch (error) {
            console.error("Failed to fetch cluster VMs:", error);
            setVmsWithAssignments([]);
        } finally {
            setLoadingVMs(false);
        }
    };

    useEffect(() => {
        if (orderedESXHosts.length > 0 && vmsWithAssignments.length > 0) {
            const esxHostOrder = new Map();
            orderedESXHosts.forEach((host, index) => {
                esxHostOrder.set(host.id, index);
            });

            const sortedVMs = [...vmsWithAssignments].sort((a, b) => {
                const aHostIndex = esxHostOrder.get(a.esxHost) ?? 999;
                const bHostIndex = esxHostOrder.get(b.esxHost) ?? 999;
                return aHostIndex - bHostIndex;
            });

            setVmsWithAssignments(sortedVMs);
        }
    }, [orderedESXHosts]);




    const handleCloseMaasConfig = () => {
        setMaasConfigDialogOpen(false);
    };

    const handleSourceClusterChange = (value) => {
        setSourceCluster(value);

        if (value) {
            const parts = value.split(":");
            const credName = parts[0];
            setSelectedVMwareCredName(credName);
        } else {
            setSelectedVMwareCredName("");
        }
    };

    const handleDestinationPCDChange = (value) => {
        setDestinationPCD(value);

        if (value) {
            const selectedPCD = pcdData.find(p => p.id === value);
            if (selectedPCD) {
                setSelectedPcdCredName(selectedPCD.openstackCredName);
                fetchOpenstackCredentialDetails(selectedPCD.openstackCredName);
            }
        } else {
            setSelectedPcdCredName("");
            setOpenstackCredData(null);
        }
    };

    const fetchOpenstackCredentialDetails = async (credName) => {
        if (!credName) return;

        setLoadingOpenstackDetails(true);
        try {
            const response = await getOpenstackCredentials(credName);
            setOpenstackCredData(response);
        } catch (error) {
            console.error("Failed to fetch OpenStack credential details:", error);
        } finally {
            setLoadingOpenstackDetails(false);
        }
    };

    // IP validation and editing functions
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



    const handleCancelEditingIP = () => {
        setEditingIpFor(null);
        setTempIpValue("");
    };

    const handleSaveIP = async (vmId: string) => {
        const vm = vmsWithAssignments.find(v => v.id === vmId);
        if (!vm || !tempIpValue.trim()) {
            handleCancelEditingIP();
            return;
        }

        if (!isValidIPAddress(tempIpValue.trim())) {
            setIpValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
            setIpValidationMessages(prev => ({ ...prev, [vmId]: 'Invalid IP address format' }));
            return;
        }

        // Set validating status
        setIpValidationStatus(prev => ({ ...prev, [vmId]: 'validating' }));
        setIpValidationMessages(prev => ({ ...prev, [vmId]: 'Validating IP address...' }));

        try {
            if (openstackCredData && vm.ip !== tempIpValue.trim()) {
                const accessInfo = await getOpenstackAccessInfo(openstackCredData);
                const validationResult = await validateOpenstackIPs({
                    ip: [tempIpValue.trim()],
                    accessInfo
                });

                const isValid = validationResult.isValid[0];
                const reason = validationResult.reason[0];

                if (!isValid) {
                    setIpValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
                    setIpValidationMessages(prev => ({ ...prev, [vmId]: reason }));
                    return;
                }
            }

            // Update the VM with new IP (only if validation passed)
            await patchVMwareMachine(vm.id, {
                spec: {
                    vms: {
                        assignedIp: tempIpValue.trim()
                    }
                }
            }, VJAILBREAK_DEFAULT_NAMESPACE);

            // Update local state with new IP
            const updatedVMs = vmsWithAssignments.map(v =>
                v.id === vmId ? { ...v, ip: tempIpValue.trim() } : v
            );
            setVmsWithAssignments(updatedVMs);

            // Set as valid immediately (no more polling needed)
            setIpValidationStatus(prev => ({ ...prev, [vmId]: 'valid' }));
            setIpValidationMessages(prev => ({ ...prev, [vmId]: 'IP validated successfully' }));

            setEditingIpFor(null);
            setTempIpValue("");

        } catch (error) {
            console.error("Failed to validate or update IP address:", error);
            setIpValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
            setIpValidationMessages(prev => ({
                ...prev,
                [vmId]: error instanceof Error ? error.message : 'Failed to validate IP address'
            }));
        }
    };

    // OS assignment handler
    const handleOSAssignment = async (vmId: string, osFamily: string) => {
        try {
            setVmOSAssignments(prev => ({ ...prev, [vmId]: osFamily }));

            await patchVMwareMachine(vmId, {
                spec: {
                    vms: {
                        osFamily: osFamily
                    }
                }
            }, VJAILBREAK_DEFAULT_NAMESPACE);

            const updatedVMs = vmsWithAssignments.map(v =>
                v.id === vmId ? { ...v, osFamily: osFamily } : v
            );
            setVmsWithAssignments(updatedVMs);

        } catch (error) {
            console.error("Failed to assign OS family:", error);
            // Revert local state on error
            setVmOSAssignments(prev => {
                const newState = { ...prev };
                delete newState[vmId];
                return newState;
            });
        }
    };


    const availableVmwareNetworks = useMemo(() => {
        if (!vmsWithAssignments.length || !selectedVMs.length) return [];

        const selectedVMsData = vmsWithAssignments.filter(vm =>
            selectedVMs.includes(vm.id));

        const extractedNetworks = selectedVMsData
            .filter(vm => vm.networks)
            .flatMap(vm => vm.networks || []);

        if (extractedNetworks.length > 0) {
            return Array.from(new Set(extractedNetworks)).sort();
        }
        return [];

    }, [vmsWithAssignments, selectedVMs]);

    const availableVmwareDatastores = useMemo(() => {
        if (!vmsWithAssignments.length || !selectedVMs.length) return [];

        const selectedVMsData = vmsWithAssignments.filter(vm =>
            selectedVMs.includes(vm.id));

        const extractedDatastores = selectedVMsData
            .filter(vm => vm.datastores)
            .flatMap(vm => vm.datastores || []);

        if (extractedDatastores.length > 0) {
            return Array.from(new Set(extractedDatastores)).sort();
        }
        return [];

    }, [vmsWithAssignments, selectedVMs]);

    // Calculate ESX host to PCD config mapping status
    const esxHostMappingStatus = useMemo(() => {
        const mappedHostsCount = orderedESXHosts.filter(host => host.pcdHostConfigName).length;
        return {
            mapped: mappedHostsCount,
            total: orderedESXHosts.length,
            fullyMapped: mappedHostsCount === orderedESXHosts.length
        };
    }, [orderedESXHosts]);

    const openstackNetworks = useMemo(() => {
        if (!openstackCredData) return [];

        const networks = openstackCredData?.status?.openstack?.networks || [];
        return networks.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
    }, [openstackCredData]);

    const openstackVolumeTypes = useMemo(() => {
        if (!openstackCredData) return [];

        const volumeTypes = openstackCredData?.status?.openstack?.volumeTypes || [];
        return volumeTypes.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
    }, [openstackCredData]);

    const openstackFlavors = useMemo(() => {
        if (!openstackCredData) return [];

        return openstackCredData?.spec?.flavors || [];
    }, [openstackCredData]);

    // Update VM flavor names when OpenStack flavors become available
    useEffect(() => {
        if (openstackFlavors.length > 0 && vmsWithAssignments.length > 0) {
            const updatedVMs = vmsWithAssignments.map(vm => {
                if (vm.targetFlavorId) {
                    const flavorObj = openstackFlavors.find(f => f.id === vm.targetFlavorId);
                    if (flavorObj && vm.flavor !== flavorObj.name) {
                        return { ...vm, flavor: flavorObj.name };
                    }
                }
                return vm;
            });

            // Only update if there are actual changes
            const hasChanges = updatedVMs.some((vm, index) =>
                vm.flavor !== vmsWithAssignments[index]?.flavor
            );

            if (hasChanges) {
                setVmsWithAssignments(updatedVMs);
            }
        }
    }, [openstackFlavors, vmsWithAssignments]);

    const handleMappingsChange = (key: string) => (value: ResourceMap[]) => {
        if (key === "networkMappings") {
            setNetworkMappings(value);
            setNetworkMappingError("");
        } else if (key === "storageMappings") {
            setStorageMappings(value);
            setStorageMappingError("");
        }
    };

    // Validate IP addresses for selected VMs
    const vmIpValidation = useMemo(() => {
        if (selectedVMs.length === 0) {
            setVmIpValidationError("");
            return { hasError: false, vmsWithoutIPs: [] };
        }

        const selectedVMsData = vmsWithAssignments.filter(vm => selectedVMs.includes(vm.id));
        const vmsWithoutIPs = selectedVMsData.filter(vm => vm.ip === "—" || !vm.ip);

        if (vmsWithoutIPs.length > 0) {
            const errorMessage = `Cannot proceed with Migration: ${vmsWithoutIPs.length} selected VM${vmsWithoutIPs.length === 1 ? '' : 's'} do not have IP addresses assigned. Please assign IP addresses to all selected VMs before continuing.`;
            setVmIpValidationError(errorMessage);
            return { hasError: true, vmsWithoutIPs };
        } else {
            setVmIpValidationError("");
            return { hasError: false, vmsWithoutIPs: [] };
        }
    }, [selectedVMs, vmsWithAssignments]);

    // Validate ESX host configs for all hosts
    const esxHostConfigValidation = useMemo(() => {
        if (orderedESXHosts.length === 0) {
            setEsxHostConfigValidationError("");
            return { hasError: false, hostsWithoutConfigs: [] };
        }

        const hostsWithoutConfigs = orderedESXHosts.filter(host => !host.pcdHostConfigName);

        if (hostsWithoutConfigs.length > 0) {
            const errorMessage = `Cannot proceed with Migration: ${hostsWithoutConfigs.length} ESXi host${hostsWithoutConfigs.length === 1 ? '' : 's'} do not have Host Config assigned. Please assign Host Config to all ESXi hosts before continuing.`;
            setEsxHostConfigValidationError(errorMessage);
            return { hasError: true, hostsWithoutConfigs };
        } else {
            setEsxHostConfigValidationError("");
            return { hasError: false, hostsWithoutConfigs: [] };
        }
    }, [orderedESXHosts]);

    // Validate OS assignment for selected powered-off VMs
    const osValidation = useMemo(() => {
        if (selectedVMs.length === 0) {
            setOsValidationError("");
            return { hasError: false, vmsWithoutOS: [] };
        }

        const selectedVMsData = vmsWithAssignments.filter(vm => selectedVMs.includes(vm.id));
        const poweredOffVMsWithoutOS = selectedVMsData.filter(vm => {
            const assignedOS = vmOSAssignments[vm.id];
            const currentOS = assignedOS || vm.osFamily;
            return vm.powerState === "powered-off" && (!currentOS || currentOS === "Unknown");
        });

        if (poweredOffVMsWithoutOS.length > 0) {
            const errorMessage = `Cannot proceed with Migration: ${poweredOffVMsWithoutOS.length} powered-off VM${poweredOffVMsWithoutOS.length === 1 ? '' : 's'} do not have Operating System assigned. Please assign OS to all powered-off VMs before continuing.`;
            setOsValidationError(errorMessage);
            return { hasError: true, vmsWithoutOS: poweredOffVMsWithoutOS };
        } else {
            setOsValidationError("");
            return { hasError: false, vmsWithoutOS: [] };
        }
    }, [selectedVMs, vmsWithAssignments, vmOSAssignments]);

    const handleSubmit = async () => {
        setSubmitting(true);

        if (selectedVMs.length > 0) {
            if (availableVmwareNetworks.some(network =>
                !networkMappings.some(mapping => mapping.source === network))) {
                setNetworkMappingError("All networks from selected VMs must be mapped");
                setSubmitting(false);
                return;
            }

            if (availableVmwareDatastores.some(datastore =>
                !storageMappings.some(mapping => mapping.source === datastore))) {
                setStorageMappingError("All datastores from selected VMs must be mapped");
                setSubmitting(false);
                return;
            }
        } else if (sourceCluster && destinationPCD) {
            alert("Please select at least one VM to migrate");
            setSubmitting(false);
            return;
        }

        try {
            const parts = sourceCluster.split(":");
            const credName = parts[0];

            const sourceItem = sourceData.find(item => item.credName === credName);
            const clusterObj = sourceItem?.clusters.find(cluster =>
                cluster.id === sourceCluster
            );
            const clusterName = clusterObj?.name || "";

            const selectedVMsData = vmsWithAssignments
                .filter(vm => selectedVMs.includes(vm.id))
                .map(vm => ({
                    vmName: vm.name,
                    esxiName: vm.esxHost
                })) as VMSequence[];

            // Create cluster mapping between VMware cluster and PCD cluster
            const selectedPCD = pcdData.find(p => p.id === destinationPCD);
            const pcdClusterName = selectedPCD?.name || "";
            const targetPCDClusterName = selectedPCD?.name;

            const clusterMapping: ClusterMapping[] = [{
                vmwareClusterName: clusterName,
                pcdClusterName: pcdClusterName
            }];

            // Update VMware hosts with their host config IDs
            const hostsToUpdate = orderedESXHosts.filter(host => host.pcdHostConfigName);

            for (const host of hostsToUpdate) {
                try {
                    // Get the host config ID from the mapping
                    const hostConfigId = esxHostToPcdMapping[host.id] || host.pcdHostConfigName;

                    if (hostConfigId) {
                        console.log(`Updating host ${host.name} with hostConfigId: ${hostConfigId}`);
                        await patchVMwareHost(host.id, hostConfigId, VJAILBREAK_DEFAULT_NAMESPACE);
                    }
                } catch (error) {
                    console.error(`Failed to update host config for ${host.name}:`, error);
                    // Continue with other hosts even if one fails
                }
            }

            // 1. Create network mapping
            const networkMappingJson = createNetworkMappingJson({
                networkMappings: networkMappings.map(mapping => ({
                    source: mapping.source,
                    target: mapping.target  // Changed from destination to target
                }))
            });
            const networkMappingResponse = await postNetworkMapping(networkMappingJson);

            // 2. Create storage mapping
            const storageMappingJson = createStorageMappingJson({
                storageMappings: storageMappings.map(mapping => ({
                    source: mapping.source,
                    target: mapping.target  // Changed from destination to target
                }))
            });
            const storageMappingResponse = await postStorageMapping(storageMappingJson);

            // 3. Create migration template
            const migrationTemplateJson = createMigrationTemplateJson({
                vmwareRef: selectedVMwareCredName,
                openstackRef: selectedPcdCredName,
                networkMapping: networkMappingResponse.metadata.name,
                storageMapping: storageMappingResponse.metadata.name,
                targetPCDClusterName: targetPCDClusterName,
            });
            const migrationTemplateResponse = await postMigrationTemplate(migrationTemplateJson);

            // 4. Create rolling migration plan with the template
            const migrationPlanJson = createRollingMigrationPlanJson({
                clusterName,
                vms: selectedVMsData,
                clusterMapping,
                bmConfigRef: {
                    name: selectedMaasConfig?.metadata.name || "",
                },
                migrationStrategy: {
                    adminInitiatedCutOver: selectedMigrationOptions.cutoverOption &&
                        params.cutoverOption === CUTOVER_TYPES.ADMIN_INITIATED,
                    healthCheckPort: "443",
                    performHealthChecks: false,
                    type: selectedMigrationOptions.dataCopyMethod ?
                        (params.dataCopyMethod as string) : "hot",
                    ...(selectedMigrationOptions.dataCopyStartTime && params.dataCopyStartTime && {
                        dataCopyStart: params.dataCopyStartTime
                    }),
                    ...(selectedMigrationOptions.cutoverOption &&
                        params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW &&
                        params.cutoverStartTime && {
                        vmCutoverStart: params.cutoverStartTime
                    }),
                    ...(selectedMigrationOptions.cutoverOption &&
                        params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW &&
                        params.cutoverEndTime && {
                        vmCutoverEnd: params.cutoverEndTime
                    }),
                },
                migrationTemplate: migrationTemplateResponse.metadata.name,
                namespace: VJAILBREAK_DEFAULT_NAMESPACE
            });

            await postRollingMigrationPlan(migrationPlanJson, VJAILBREAK_DEFAULT_NAMESPACE);

            console.log("Submitted rolling migration plan", migrationPlanJson);
            onClose();
            navigate("/dashboard?tab=clusterconversions");
        } catch (error) {
            console.error("Failed to submit rolling migration plan:", error);
            const errorMessage = error instanceof Error ? error.message : "Unknown error";
            alert(`Failed to submit rolling migration plan: ${errorMessage}`);
        } finally {
            setSubmitting(false);
        }
    };

    const handleClose = () => {
        if (!submitting) {
            onClose();
        }
    };

    const isSubmitDisabled = useMemo(() => {
        const basicRequirementsMissing = !sourceCluster ||
            !destinationPCD ||
            !selectedMaasConfig ||
            !selectedVMs.length ||
            submitting;


        const mappingsValid = !(availableVmwareNetworks.some(network =>
            !networkMappings.some(mapping => mapping.source === network)) ||
            availableVmwareDatastores.some(datastore =>
                !storageMappings.some(mapping => mapping.source === datastore)));

        // Migration options validation
        const migrationOptionValidated = Object.keys(selectedMigrationOptions).every((key) => {
            if (selectedMigrationOptions[key]) {
                if (
                    key === "cutoverOption" &&
                    params.cutoverOption === CUTOVER_TYPES.TIME_WINDOW
                ) {
                    return (
                        params.cutoverStartTime &&
                        params.cutoverEndTime &&
                        !fieldErrors["cutoverStartTime"] &&
                        !fieldErrors["cutoverEndTime"]
                    )
                }
                return params?.[key] && !fieldErrors[key];
            }
            return true;
        });

        // PCD host config validation - ensure all selected ESX hosts have PCD host configs assigned
        const pcdHostConfigValid = selectedESXHosts.length === 0 ||
            selectedESXHosts.every(hostId => {
                const host = orderedESXHosts.find(h => h.id === hostId);
                return host?.pcdHostConfigName;
            });

        // ESX host config validation - ensure all ESX hosts have host configs assigned
        const esxHostConfigValid = !esxHostConfigValidation.hasError;

        // IP validation - ensure all selected VMs have IP addresses assigned
        const ipValidationPassed = !vmIpValidation.hasError;

        // OS validation - ensure all selected powered-off VMs have OS assigned
        const osValidationPassed = !osValidation.hasError;

        return basicRequirementsMissing || !mappingsValid || !migrationOptionValidated || !pcdHostConfigValid || !esxHostConfigValid || !ipValidationPassed || !osValidationPassed;
    }, [
        sourceCluster,
        destinationPCD,
        selectedMaasConfig,
        submitting,
        selectedVMs,
        availableVmwareNetworks,
        networkMappings,
        availableVmwareDatastores,
        storageMappings,
        selectedMigrationOptions,
        params,
        fieldErrors,
        selectedESXHosts,
        orderedESXHosts,
        vmIpValidation.hasError,
        esxHostConfigValidation.hasError,
        osValidation.hasError
    ]);

    useKeyboardSubmit({
        open,
        isSubmitDisabled: isSubmitDisabled,
        onSubmit: handleSubmit,
        onClose: handleClose
    });

    const handleViewMaasConfig = () => {
        setMaasDetailsModalOpen(true);
    };

    const handleCloseMaasDetailsModal = () => {
        setMaasDetailsModalOpen(false);
    };

    const handleOpenPcdHostConfigDialog = () => {
        if (selectedESXHosts.length === 0) return;
        setPcdHostConfigDialogOpen(true);
    };

    const handleClosePcdHostConfigDialog = () => {
        setPcdHostConfigDialogOpen(false);
        setSelectedPcdHostConfig("");
    };

    const handlePcdHostConfigChange = (event) => {
        setSelectedPcdHostConfig(event.target.value);
    };

    const handleApplyPcdHostConfig = async () => {
        if (!selectedPcdHostConfig) {
            handleClosePcdHostConfigDialog();
            return;
        }

        setUpdatingPcdMapping(true);

        try {
            const availablePcdHostConfigs = openstackCredData?.spec?.pcdHostConfig || [];
            const selectedPcdConfig = availablePcdHostConfigs.find(config => config.id === selectedPcdHostConfig);
            const pcdConfigName = selectedPcdConfig ? selectedPcdConfig.name : selectedPcdHostConfig;

            // Update the ESX hosts with the selected host config
            const updatedESXHosts = orderedESXHosts.map(host => {
                if (selectedESXHosts.includes(host.id)) {
                    return {
                        ...host,
                        pcdHostConfigName: pcdConfigName
                    };
                }
                return host;
            });

            setOrderedESXHosts(updatedESXHosts);

            // Update the mapping record
            const newMapping = { ...esxHostToPcdMapping };
            selectedESXHosts.forEach(hostId => {
                newMapping[hostId as string] = selectedPcdHostConfig;
            });
            setEsxHostToPcdMapping(newMapping);

            handleClosePcdHostConfigDialog();
        } catch (error) {
            console.error("Error updating PCD host config mapping:", error);
        } finally {
            setUpdatingPcdMapping(false);
        }
    };

    // Define VM columns inside component to access state
    const vmColumns: GridColDef[] = [
        {
            field: "name",
            headerName: "VM Name",
            flex: 1.5,
            hideable: false, // Always show VM name
            renderCell: (params) => (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Tooltip title={params.row.powerState === "powered-on" ? "Powered On" : "Powered Off"}>
                        <CdsIconWrapper>
                            {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                            {/* @ts-ignore */}
                            <cds-icon shape="vm" size="md" badge={params.row.powerState === "powered-on" ? "success" : "danger"}></cds-icon>
                        </CdsIconWrapper>
                    </Tooltip>
                    <Box>{params.value}</Box>
                </Box>
            ),
        },
        {
            field: "ip",
            headerName: "Current IP",
            flex: 1,
            hideable: true,
            renderCell: (params) => {
                const vmId = params.row.id;
                const isSelected = selectedVMs.includes(vmId);
                const isEditing = editingIpFor === vmId;
                const validationStatus = ipValidationStatus[vmId];
                const validationMessage = ipValidationMessages[vmId];
                const currentIp = params.value || "—";

                const getValidationIcon = () => {
                    switch (validationStatus) {
                        case 'valid':
                            return <Tooltip title={validationMessage}><CheckCircleIcon color="success" sx={{ ml: 1, fontSize: 16 }} /></Tooltip>;
                        case 'invalid':
                            return <Tooltip title={validationMessage}><ErrorIcon color="error" sx={{ ml: 1, fontSize: 16 }} /></Tooltip>;
                        case 'pending':
                            return <Tooltip title={validationMessage}><WarningIcon color="warning" sx={{ ml: 1, fontSize: 16 }} /></Tooltip>;
                        case 'validating':
                            return <Tooltip title={validationMessage}><CircularProgress size={20} sx={{ ml: 1, display: 'flex', alignItems: 'center' }} /></Tooltip>;
                        default:
                            return null;
                    }
                };

                // Show input field if VM is selected or being edited
                if (isSelected || isEditing) {
                    return (
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: '100%', height: '100%', }}>
                            <TextField
                                value={isEditing ? tempIpValue : currentIp === "—" ? "" : currentIp}
                                onChange={(e) => {
                                    if (!isEditing) {
                                        setTempIpValue(e.target.value);
                                        setEditingIpFor(vmId);
                                    } else {
                                        setTempIpValue(e.target.value);
                                    }
                                }}
                                onBlur={() => {
                                    if (isEditing) {
                                        handleSaveIP(vmId);
                                    }
                                }}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter') {
                                        if (isEditing) {
                                            handleSaveIP(vmId);
                                        } else {
                                            setEditingIpFor(vmId);
                                            handleSaveIP(vmId);
                                        }
                                    } else if (e.key === 'Escape') {
                                        handleCancelEditingIP();
                                    }
                                }}
                                size="small"
                                placeholder="Enter IP address"
                                sx={{
                                    '& .MuiInputBase-input': {
                                        fontSize: '0.875rem',
                                        padding: '4px 8px'
                                    }
                                }}
                                slotProps={{
                                    input: {
                                        endAdornment: getValidationIcon()
                                    }
                                }}
                            />
                        </Box>
                    );
                }

                // Show read-only display for unselected VMs
                return (
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
                        <Typography variant="body2">
                            {currentIp}
                        </Typography>
                        {getValidationIcon()}
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
                const isSelected = selectedVMs.includes(vmId);
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

                // Show OS with icon for assigned/detected OS
                let displayValue = currentOsFamily || "Unknown";
                let icon: React.ReactNode = null;

                if (currentOsFamily && currentOsFamily.toLowerCase().includes("windows")) {
                    displayValue = "Windows";
                    icon = <img src={WindowsIcon} alt="Windows" style={{ width: 20, height: 20 }} />;
                } else if (currentOsFamily && currentOsFamily.toLowerCase().includes("linux")) {
                    displayValue = "Linux";
                    icon = <img src={LinuxIcon} alt="Linux" style={{ width: 20, height: 20 }} />;
                } else if (currentOsFamily && currentOsFamily !== "Unknown") {
                    displayValue = "Other";
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
                            {powerState === "powered-off" && (!currentOsFamily || currentOsFamily === "Unknown") && (
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
            flex: 1,
            hideable: true,
            valueGetter: (value) => value || "—",
        },
        {
            field: "cpu",
            headerName: "CPU",
            flex: 0.3,
            hideable: true,
            valueGetter: (value) => value || "- ",
        },
        {
            field: "memory",
            headerName: "Memory (MB)",
            flex: 0.8,
            hideable: true,
            valueGetter: (value) => value || "—",
        },
        {
            field: "esxHost",
            headerName: "ESX Host",
            flex: 1,
            hideable: true,
            valueGetter: (value) => value || "—",
        },
        {
            field: "flavor",
            headerName: "Flavor",
            flex: 1,
            hideable: true,
            renderCell: (params) => {
                const vmId = params.row.id;
                const isSelected = selectedVMs.includes(vmId);
                const currentFlavor = params.value || "auto-assign";

                if (isSelected) {
                    return (
                        <Box sx={{ display: 'flex', alignItems: 'center', height: '100%', width: '100%' }}>
                            <Select
                                size="small"
                                value={(() => {
                                    if (currentFlavor === "auto-assign") return "auto-assign";
                                    const flavorByName = openstackFlavors.find(f => f.name === currentFlavor);
                                    const flavorById = openstackFlavors.find(f => f.id === currentFlavor);
                                    return flavorByName?.id || flavorById?.id || currentFlavor;
                                })()}
                                onChange={(e) => handleIndividualFlavorChange(vmId, e.target.value)}
                                displayEmpty
                                sx={{
                                    minWidth: 120,
                                    width: '100%',
                                    '& .MuiSelect-select': {
                                        padding: '4px 8px',
                                        fontSize: '0.875rem'
                                    }
                                }}
                            >
                                <MenuItem value="auto-assign">
                                    <Typography variant="body2">Auto Assign</Typography>
                                </MenuItem>
                                {openstackFlavors.map((flavor) => (
                                    <MenuItem key={flavor.id} value={flavor.id}>
                                        <Typography variant="body2">{flavor.name}</Typography>
                                    </MenuItem>
                                ))}
                            </Select>
                        </Box>
                    );
                }

                return (
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
                        <Typography variant="body2">
                            {currentFlavor}
                        </Typography>
                    </Box>
                );
            },
        },
        {
            field: "powerState",
            headerName: "Power State",
            hideable: true,
            flex: 0.8,
            valueGetter: (value) => value || "—",
        }
    ];

    const handleOpenBulkEditDialog = () => {
        if (selectedVMs.length === 0) return;

        // Initialize bulk edit state
        const selectedVMsData = vmsWithAssignments.filter(vm => selectedVMs.includes(vm.id));
        const initialIPs: Record<string, string> = {};
        const initialStatus: Record<string, 'empty' | 'valid' | 'invalid' | 'validating'> = {};

        selectedVMsData.forEach(vm => {
            initialIPs[vm.id] = vm.ip === "—" ? "" : vm.ip;
            initialStatus[vm.id] = vm.ip === "—" ? 'empty' : 'valid';
        });

        setBulkEditIPs(initialIPs);
        setBulkValidationStatus(initialStatus);
        setBulkValidationMessages({});
        setBulkEditDialogOpen(true);
    };

    const handleCloseBulkEditDialog = () => {
        setBulkEditDialogOpen(false);
        setBulkEditIPs({});
        setBulkValidationStatus({});
        setBulkValidationMessages({});
    };

    const handleBulkIpChange = (vmId: string, value: string) => {
        setBulkEditIPs(prev => ({ ...prev, [vmId]: value }));

        if (!value.trim()) {
            setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'empty' }));
            setBulkValidationMessages(prev => ({ ...prev, [vmId]: '' }));
        } else if (!isValidIPAddress(value.trim())) {
            setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
            setBulkValidationMessages(prev => ({ ...prev, [vmId]: 'Invalid IP format' }));
        } else {
            setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'empty' }));
            setBulkValidationMessages(prev => ({ ...prev, [vmId]: '' }));
        }
    };

    const handleClearAllIPs = () => {
        const clearedIPs: Record<string, string> = {};
        const clearedStatus: Record<string, 'empty' | 'valid' | 'invalid' | 'validating'> = {};

        Object.keys(bulkEditIPs).forEach(vmId => {
            clearedIPs[vmId] = "";
            clearedStatus[vmId] = 'empty';
        });

        setBulkEditIPs(clearedIPs);
        setBulkValidationStatus(clearedStatus);
        setBulkValidationMessages({});
    };

    const handleApplyBulkIPs = async () => {
        const ipsToApply = Object.entries(bulkEditIPs).filter(([, ip]) => ip.trim() !== "");
        if (ipsToApply.length === 0) return;

        setAssigningIPs(true);

        try {
            // NEW: Batch validation before applying any changes
            if (openstackCredData) {
                const accessInfo = await getOpenstackAccessInfo(openstackCredData);
                const ipList = ipsToApply.map(([, ip]) => ip.trim());

                setBulkValidationStatus(prev => {
                    const newStatus = { ...prev };
                    ipsToApply.forEach(([vmId]) => {
                        newStatus[vmId] = 'validating';
                    });
                    return newStatus;
                });

                const validationResult = await validateOpenstackIPs({
                    ip: ipList,
                    accessInfo
                });

                // Process validation results
                const validIPs: Array<[string, string]> = [];
                let hasInvalidIPs = false;

                ipsToApply.forEach(([vmId, ip], index) => {
                    const isValid = validationResult.isValid[index];
                    const reason = validationResult.reason[index];

                    if (isValid) {
                        validIPs.push([vmId, ip.trim()]);
                        setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'valid' }));
                        setBulkValidationMessages(prev => ({ ...prev, [vmId]: 'Valid' }));
                    } else {
                        hasInvalidIPs = true;
                        setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
                        setBulkValidationMessages(prev => ({ ...prev, [vmId]: reason }));
                    }
                });

                // Only proceed if ALL IPs are valid
                if (hasInvalidIPs) {
                    setAssigningIPs(false);
                    return;
                }

                // Apply only the valid IPs to VMs (all IPs are valid at this point)
                const updatePromises = validIPs.map(async ([vmId, ip]) => {
                    try {
                        await patchVMwareMachine(vmId, {
                            spec: {
                                vms: {
                                    assignedIp: ip
                                }
                            }
                        }, VJAILBREAK_DEFAULT_NAMESPACE);
                        return { success: true, vmId, ip };
                    } catch (error) {
                        setBulkValidationStatus(prev => ({ ...prev, [vmId]: 'invalid' }));
                        setBulkValidationMessages(prev => ({
                            ...prev,
                            [vmId]: error instanceof Error ? error.message : 'Failed to apply IP'
                        }));
                        return { success: false, vmId, error };
                    }
                });

                const results = await Promise.all(updatePromises);

                // Check if any updates failed
                const failedUpdates = results.filter(result => !result.success);
                if (failedUpdates.length > 0) {
                    setAssigningIPs(false);
                    return; // Don't close modal if any updates failed
                }

                // Update local VM state only if all validations and updates succeeded
                const updatedVMs = vmsWithAssignments.map(vm => {
                    const newIp = bulkEditIPs[vm.id];
                    return newIp && newIp.trim() ? { ...vm, ip: newIp.trim() } : vm;
                });
                setVmsWithAssignments(updatedVMs);

                // Transfer validation status from bulk edit to individual IP validation state
                const newIpValidationStatus = { ...ipValidationStatus };
                const newIpValidationMessages = { ...ipValidationMessages };

                validIPs.forEach(([vmId]) => {
                    newIpValidationStatus[vmId] = 'valid';
                    newIpValidationMessages[vmId] = 'IP validated and applied successfully';
                });

                setIpValidationStatus(newIpValidationStatus);
                setIpValidationMessages(newIpValidationMessages);

                handleCloseBulkEditDialog();
            }

        } catch (error) {
            console.error("Error in bulk IP validation/assignment:", error);
        } finally {
            setAssigningIPs(false);
        }
    };

    // Update the toolbar handler
    const handleEditIPs = () => {
        handleOpenBulkEditDialog();
    };

    // Flavor assignment handlers
    const handleOpenFlavorDialog = () => {
        if (selectedVMs.length === 0) return;
        setFlavorDialogOpen(true);
    };

    const handleCloseFlavorDialog = () => {
        setFlavorDialogOpen(false);
        setSelectedFlavor("");
    };

    const handleFlavorChange = (event) => {
        setSelectedFlavor(event.target.value);
    };

    const handleIndividualFlavorChange = async (vmId: string, flavorValue: string) => {
        try {
            const isAutoAssign = flavorValue === "auto-assign";
            const selectedFlavorObj = !isAutoAssign ? openstackFlavors.find(f => f.id === flavorValue) : null;
            const flavorName = isAutoAssign ? "auto-assign" : (selectedFlavorObj ? selectedFlavorObj.name : flavorValue);

            // Update VM via API
            const payload = {
                spec: {
                    targetFlavorId: isAutoAssign ? "" : flavorValue
                }
            };

            await patchVMwareMachine(vmId, payload, VJAILBREAK_DEFAULT_NAMESPACE);

            // Update local state
            const updatedVMs = vmsWithAssignments.map(vm => {
                if (vm.id === vmId) {
                    return {
                        ...vm,
                        flavor: flavorName,
                        targetFlavorId: isAutoAssign ? "" : flavorValue
                    };
                }
                return vm;
            });
            setVmsWithAssignments(updatedVMs);

            console.log(`Successfully assigned flavor "${flavorName}" to VM ${vmId}`);

        } catch (error) {
            console.error(`Failed to update flavor for VM ${vmId}:`, error);
            alert(`Failed to assign flavor to VM: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
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

            // Update VMs via API
            const updatePromises = selectedVMs.map(async (vmId) => {
                try {
                    const payload = {
                        spec: {
                            targetFlavorId: isAutoAssign ? "" : selectedFlavor
                        }
                    };

                    await patchVMwareMachine(vmId as string, payload, VJAILBREAK_DEFAULT_NAMESPACE);
                    return { success: true, vmId };
                } catch (error) {
                    console.error(`Failed to update flavor for VM ${vmId}:`, error);
                    return { success: false, vmId, error };
                }
            });

            const results = await Promise.all(updatePromises);
            const failedUpdates = results.filter(result => !result.success);

            if (failedUpdates.length > 0) {
                console.error(`Failed to update flavor for ${failedUpdates.length} VMs`);
                alert(`Failed to assign flavor to ${failedUpdates.length} VM${failedUpdates.length > 1 ? 's' : ''}`);
            } else {
                // Update local state only if all API calls succeeded
                const updatedVMs = vmsWithAssignments.map(vm => {
                    if (selectedVMs.includes(vm.id)) {
                        return {
                            ...vm,
                            flavor: flavorName,
                            targetFlavorId: isAutoAssign ? "" : selectedFlavor
                        };
                    }
                    return vm;
                });
                setVmsWithAssignments(updatedVMs);

                const actionText = isAutoAssign ? "cleared flavor assignment for" : "assigned flavor to";
                console.log(`Successfully ${actionText} ${selectedVMs.length} VM${selectedVMs.length > 1 ? 's' : ''}`);

                // Refresh VM list to get updated flavor information from API
                await fetchClusterVMs();
            }

            handleCloseFlavorDialog();
        } catch (error) {
            console.error("Error updating flavors:", error);
            alert("Failed to assign flavor to VMs");
        } finally {
            setUpdating(false);
        }
    };

    return (
        <>
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
            <StyledDrawer
                anchor="right"
                open={open}
                onClose={handleClose}
                ModalProps={{ keepMounted: false }}
            >
                <Header title="Cluster Conversion " />


                <DrawerContent>
                    <Box sx={{ display: "grid", gap: 4 }}>
                        <SourceDestinationClusterSelection
                            onChange={() => () => { }}
                            errors={{}}
                            stepNumber="1"
                            stepLabel="Source & Destination"
                            onVmwareClusterChange={handleSourceClusterChange}
                            onPcdClusterChange={handleDestinationPCDChange}
                            vmwareCluster={sourceCluster}
                            pcdCluster={destinationPCD}
                            loadingVMware={loading}
                            loadingPCD={loadingPCD}
                        />
                        <Box>
                            <Step stepNumber="2" label="MAAS Config (Verify the configuration)" />
                            <Box sx={{ ml: 5, mt: 1 }}>
                                {loadingMaasConfig ? (
                                    <Typography variant="body2">Loading MAAS Config...</Typography>
                                ) : maasConfigs.length === 0 ? (
                                    <Typography variant="body2">No MAAS Config available</Typography>
                                ) : (
                                    <Typography
                                        variant="subtitle2"
                                        component="a"
                                        sx={{
                                            color: 'primary.main',
                                            textDecoration: 'underline',
                                            cursor: 'pointer',
                                            fontWeight: "500"
                                        }}
                                        onClick={handleViewMaasConfig}
                                    >
                                        View MAAS Config Details
                                    </Typography>
                                )}
                            </Box>
                        </Box>

                        <Box>
                            <Step stepNumber="3" label="ESXi Hosts" />
                            <Box sx={{ ml: 5, mt: 2 }}>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                                    <Typography variant="body2" color="text.secondary">
                                        Select ESXi hosts and assign PCD host configurations
                                    </Typography>
                                    {esxHostMappingStatus.fullyMapped && esxHostMappingStatus.total > 0 ? (
                                        <Typography variant="body2" color="success.main">
                                            All hosts mapped ✓
                                        </Typography>
                                    ) :
                                        <Typography variant="body2" color="warning.main">
                                            {esxHostMappingStatus.mapped} of {esxHostMappingStatus.total} hosts unmapped
                                        </Typography>
                                    }
                                </Box>
                                <Paper sx={{ width: "100%", height: 389 }}>
                                    <DataGrid
                                        rows={orderedESXHosts}
                                        columns={esxColumns}
                                        initialState={{
                                            pagination: { paginationModel },
                                            columns: {
                                                columnVisibilityModel: {}
                                            }
                                        }}
                                        pageSizeOptions={[5, 10, 25]}
                                        rowHeight={45}
                                        checkboxSelection
                                        onRowSelectionModelChange={setSelectedESXHosts}
                                        rowSelectionModel={selectedESXHosts}
                                        slots={{
                                            toolbar: (props) => (
                                                <CustomESXToolbarWithActions
                                                    {...props}
                                                    rowSelectionModel={selectedESXHosts}
                                                    onAddPcdHost={handleOpenPcdHostConfigDialog}
                                                />
                                            )
                                        }}
                                        disableColumnMenu
                                        disableColumnFilter
                                        loading={loadingHosts}
                                    />
                                </Paper>
                                {esxHostConfigValidationError && (
                                    <Alert severity="warning" sx={{ mt: 2 }}>
                                        {esxHostConfigValidationError}
                                    </Alert>
                                )}
                            </Box>
                        </Box>

                        <Box>
                            <Step stepNumber="4" label="Select Virtual Machines to Migrate" />
                            <Box sx={{ ml: 5, mt: 2 }}>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                                    <Typography variant="body2" color="text.secondary">
                                        Select VMs to migrate
                                    </Typography>
                                </Box>
                                <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block', mb: 1 }}>
                                    💡 Tip: Powered-off VMs require IP Address and OS assignment for proper migration configuration
                                </Typography>
                                <Paper sx={{ width: "100%", height: 389 }}>
                                    <DataGrid
                                        rows={vmsWithAssignments}
                                        columns={vmColumns}
                                        initialState={{
                                            pagination: { paginationModel },
                                            columns: {
                                                columnVisibilityModel: {}
                                            }
                                        }}
                                        pageSizeOptions={[5, 10, 25]}
                                        rowHeight={45}
                                        checkboxSelection
                                        onRowSelectionModelChange={setSelectedVMs}
                                        rowSelectionModel={selectedVMs}
                                        slots={{
                                            toolbar: (props) => (
                                                <CustomToolbarWithActions
                                                    {...props}
                                                    rowSelectionModel={selectedVMs}
                                                    onEditIPs={handleEditIPs}
                                                    onAssignFlavor={handleOpenFlavorDialog}
                                                />
                                            ),
                                            noRowsOverlay: () => (
                                                <Box sx={{
                                                    display: 'flex',
                                                    flexDirection: 'column',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    height: '100%',
                                                    p: 2
                                                }}>
                                                    <Typography variant="body1" color="text.secondary" align="center" sx={{ mb: 1 }}>
                                                        {loadingVMs ? 'Loading VMs...' : 'No VMs found for the selected cluster.'}
                                                    </Typography>
                                                </Box>
                                            )
                                        }}
                                        disableColumnFilter
                                        disableRowSelectionOnClick
                                        loading={loadingVMs}
                                    />
                                </Paper>
                                {vmIpValidationError && (
                                    <Alert severity="warning" sx={{ mt: 2 }}>
                                        {vmIpValidationError}
                                    </Alert>
                                )}
                                {osValidationError && (
                                    <Alert severity="warning" sx={{ mt: 2 }}>
                                        {osValidationError}
                                    </Alert>
                                )}
                            </Box>
                        </Box>

                        <Box>
                            {sourceCluster && destinationPCD ? (
                                <>
                                    <NetworkAndStorageMappingStep
                                        vmwareNetworks={availableVmwareNetworks}
                                        vmWareStorage={availableVmwareDatastores}
                                        openstackNetworks={openstackNetworks}
                                        openstackStorage={openstackVolumeTypes}
                                        params={{
                                            networkMappings: networkMappings,
                                            storageMappings: storageMappings
                                        }}
                                        onChange={handleMappingsChange}
                                        networkMappingError={networkMappingError}
                                        storageMappingError={storageMappingError}
                                        stepNumber="5"
                                        loading={loadingOpenstackDetails}
                                    />
                                    <MigrationOptions
                                        stepNumber="6"
                                        params={params}
                                        onChange={getParamsUpdater}
                                        selectedMigrationOptions={selectedMigrationOptions}
                                        updateSelectedMigrationOptions={updateSelectedMigrationOptions}
                                        errors={fieldErrors}
                                        getErrorsUpdater={getFieldErrorsUpdater}
                                    />
                                </>


                            ) : (
                                <Typography variant="body2" color="text.secondary">
                                    Please select both source cluster and destination PCD to configure mappings.
                                </Typography>
                            )}
                        </Box>
                    </Box>
                </DrawerContent>
                <Footer
                    submitButtonLabel="Run"
                    onClose={handleClose}
                    onSubmit={handleSubmit}
                    disableSubmit={isSubmitDisabled}
                    submitting={submitting}
                />

                <MaasConfigDialog
                    open={maasConfigDialogOpen}
                    onClose={handleCloseMaasConfig}
                    aria-labelledby="maas-config-dialog-title"
                >
                    <DialogTitle id="maas-config-dialog-title">
                        <Typography variant="h6">ESXi - MAAS Configuration</Typography>
                    </DialogTitle>
                    <DialogContent dividers>
                        {loadingMaasConfig ? (
                            <Typography>Loading configuration details...</Typography>
                        ) : !selectedMaasConfig ? (
                            <Typography>No configuration available</Typography>
                        ) : (
                            <>
                                <ConfigSection>
                                    <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 500 }}>Provider Configuration</Typography>
                                    <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 3, ml: 1 }}>
                                        <ConfigField>
                                            <FieldLabel>Provider Type:</FieldLabel>
                                            <FieldValue>{selectedMaasConfig.spec.providerType}</FieldValue>
                                        </ConfigField>
                                        <ConfigField>
                                            <FieldLabel>MAAS URL:</FieldLabel>
                                            <FieldValue>{selectedMaasConfig.spec.apiUrl}</FieldValue>
                                        </ConfigField>
                                        <ConfigField>
                                            <FieldLabel>Insecure:</FieldLabel>
                                            <FieldValue>{selectedMaasConfig.spec.insecure ? "Yes" : "No"}</FieldValue>
                                        </ConfigField>
                                        {selectedMaasConfig.spec.os && (
                                            <ConfigField>
                                                <FieldLabel>OS:</FieldLabel>
                                                <FieldValue>{selectedMaasConfig.spec.os}</FieldValue>
                                            </ConfigField>
                                        )}
                                        <ConfigField>
                                            <FieldLabel>Status:</FieldLabel>
                                            <FieldValue>
                                                {selectedMaasConfig.status?.validationStatus || "Pending validation"}
                                            </FieldValue>
                                        </ConfigField>
                                        {selectedMaasConfig.status?.validationMessage && (
                                            <ConfigField>
                                                <FieldLabel>Validation Message:</FieldLabel>
                                                <FieldValue>{selectedMaasConfig.status.validationMessage}</FieldValue>
                                            </ConfigField>
                                        )}
                                    </Box>
                                </ConfigSection>

                                {selectedMaasConfig.spec.userDataSecretRef && (
                                    <ConfigSection>
                                        <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 500 }}>Cloud-Init Configuration</Typography>
                                        <Typography variant="caption" sx={{ mb: 1, display: 'block', color: 'text.secondary' }}>
                                            User data is stored in a secret: {selectedMaasConfig.spec.userDataSecretRef.name}
                                        </Typography>
                                        <CodeEditorContainer>
                                            <SyntaxHighlighter
                                                language="yaml"
                                                style={oneLight}
                                                showLineNumbers
                                                wrapLongLines
                                                customStyle={{
                                                    margin: 0,
                                                    maxHeight: '100%',
                                                }}
                                            >
                                                {`# Cloud-init configuration is stored in Kubernetes Secret: 
# ${selectedMaasConfig.spec.userDataSecretRef.name}
# in namespace: ${selectedMaasConfig.spec.userDataSecretRef.namespace || VJAILBREAK_DEFAULT_NAMESPACE}

# The cloud-init configuration includes:
# - package updates and installations
# - configuration files
# - commands to run on startup
# - network configuration
# - and other system setup parameters

# This will be used when provisioning ESXi hosts in the bare metal environment.`}
                                            </SyntaxHighlighter>
                                        </CodeEditorContainer>
                                    </ConfigSection>
                                )}

                                <ConfigSection>
                                    <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 500 }}>Resource Information</Typography>
                                    <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 3, ml: 1 }}>
                                        <ConfigField>
                                            <FieldLabel>Name:</FieldLabel>
                                            <FieldValue>{selectedMaasConfig.metadata.name}</FieldValue>
                                        </ConfigField>
                                        <ConfigField>
                                            <FieldLabel>Namespace:</FieldLabel>
                                            <FieldValue>{selectedMaasConfig.metadata.namespace}</FieldValue>
                                        </ConfigField>
                                        <ConfigField>
                                            <FieldLabel>Created:</FieldLabel>
                                            <FieldValue>
                                                {new Date(selectedMaasConfig.metadata.creationTimestamp).toLocaleString()}
                                            </FieldValue>
                                        </ConfigField>
                                    </Box>
                                </ConfigSection>
                            </>
                        )}
                    </DialogContent>
                    <DialogActions>
                        <Button variant="contained" onClick={handleCloseMaasConfig}>
                            Close
                        </Button>
                    </DialogActions>
                </MaasConfigDialog>

                {
                    maasConfigs && maasConfigs.length > 0 && (
                        <MaasConfigDetailsModal
                            open={maasDetailsModalOpen}
                            onClose={handleCloseMaasDetailsModal}
                            configName={maasConfigs[0].metadata.name}
                            namespace={maasConfigs[0].metadata.namespace}
                        />
                    )
                }

                {/* PCD Host Config Assignment Dialog */}
                <Dialog
                    open={pcdHostConfigDialogOpen}
                    onClose={handleClosePcdHostConfigDialog}
                    fullWidth
                    maxWidth="sm"
                >
                    <DialogTitle>
                        Assign Host Config to {selectedESXHosts.length} {selectedESXHosts.length === 1 ? 'ESXi Host' : 'ESXi Hosts'}
                    </DialogTitle>
                    <DialogContent>
                        <Box sx={{ my: 2 }}>
                            <Typography variant="body2" gutterBottom>
                                Select Host Configuration
                            </Typography>
                            <Select
                                fullWidth
                                value={selectedPcdHostConfig}
                                onChange={handlePcdHostConfigChange}
                                size="small"
                                sx={{ mt: 1 }}
                                displayEmpty
                            >
                                <MenuItem value="">
                                    <em>Select a  host configuration</em>
                                </MenuItem>
                                {(openstackCredData?.spec?.pcdHostConfig || []).map((config) => (
                                    <MenuItem key={config.id} value={config.id}>
                                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                            <Typography variant="body1">{config.name}</Typography>
                                            <Typography variant="caption" color="text.secondary">
                                                Management Interface: {config.mgmtInterface}
                                            </Typography>
                                        </Box>
                                    </MenuItem>
                                ))}
                            </Select>
                        </Box>
                    </DialogContent>
                    <DialogActions>
                        <Button onClick={handleClosePcdHostConfigDialog}>Cancel</Button>
                        <Button
                            onClick={handleApplyPcdHostConfig}
                            variant="contained"
                            color="primary"
                            disabled={!selectedPcdHostConfig || updatingPcdMapping}
                        >
                            {updatingPcdMapping ? "Applying..." : "Apply to selected hosts"}
                        </Button>
                    </DialogActions>
                </Dialog>

                {/* Bulk IP Editor Dialog */}
                <Dialog
                    open={bulkEditDialogOpen}
                    onClose={handleCloseBulkEditDialog}
                    maxWidth="md"
                >
                    <DialogTitle>
                        Edit IP Addresses for {selectedVMs.length} {selectedVMs.length === 1 ? 'VM' : 'VMs'}
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
                                {Object.entries(bulkEditIPs).map(([vmId, ip]) => {
                                    const vm = vmsWithAssignments.find(v => v.id === vmId);
                                    if (!vm) return null;

                                    const status = bulkValidationStatus[vmId];
                                    const message = bulkValidationMessages[vmId];

                                    return (
                                        <Box key={vmId} sx={{ mb: 2, display: 'flex', alignItems: 'center' }}>
                                            <Box sx={{ width: 400, flexShrink: 0, pr: 3 }}>
                                                <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                                                    {vm.name}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    Current: {vm.ip}
                                                </Typography>
                                            </Box>
                                            <Box sx={{ width: 400, height: 60, display: 'flex', alignItems: 'center', gap: 2 }}>
                                                <TextField
                                                    value={ip}
                                                    onChange={(e) => handleBulkIpChange(vmId, e.target.value)}
                                                    placeholder="Enter IP address"
                                                    size="small"
                                                    sx={{ width: 260 }}
                                                    error={status === 'invalid'}
                                                    helperText={message}
                                                />
                                                <Box sx={{ width: 24, display: 'flex' }}>
                                                    {status === 'validating' && <CircularProgress size={20} />}
                                                    {status === 'valid' && <CheckCircleIcon color="success" fontSize="small" />}
                                                    {status === 'invalid' && <ErrorIcon color="error" fontSize="small" sx={{ mb: 3 }} />}
                                                </Box>
                                            </Box>
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
                            disabled={Object.values(bulkEditIPs).every(ip => !ip.trim()) || assigningIPs}
                        >
                            {assigningIPs ? "Applying..." : "Apply Changes"}
                        </Button>
                    </DialogActions>
                </Dialog>



                {/* Flavor Assignment Dialog */}
                <Dialog
                    open={flavorDialogOpen}
                    onClose={handleCloseFlavorDialog}
                    fullWidth
                    maxWidth="sm"
                >
                    <DialogTitle>
                        Assign Flavor to {selectedVMs.length} {selectedVMs.length === 1 ? 'VM' : 'VMs'}
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
            </StyledDrawer>
        </>
    );
} 