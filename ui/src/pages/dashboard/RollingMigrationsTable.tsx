import { useState, useMemo } from "react";
import {
    Box,
    Typography,
    Chip,
    Drawer,
    styled,
    Button,
    LinearProgress,
    IconButton
} from "@mui/material";
import {
    DataGrid,
    GridColDef,
    GridToolbarContainer,
    GridRowSelectionModel
} from "@mui/x-data-grid";
import VisibilityIcon from '@mui/icons-material/Visibility';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import { QueryObserverResult } from "@tanstack/react-query";
import { RefetchOptions } from "@tanstack/react-query";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import { ReactElement } from "react";
import WarningIcon from '@mui/icons-material/Warning';
import ConfirmationDialog from "src/components/dialogs/ConfirmationDialog";
import { deleteClusterMigration } from "src/api/clustermigrations/clustermigrations";
import { useQueryClient } from "@tanstack/react-query";
import { CLUSTER_MIGRATIONS_QUERY_KEY } from "src/hooks/api/useClusterMigrationsQuery";

// Import CDS icons
import "@cds/core/icon/register.js";
import { ClarityIcons, buildingIcon, clusterIcon, hostIcon, vmIcon } from "@cds/core/icon";
import { ESXHost, ESXIMigration } from "src/api/esximigrations/model";
import { Migration, Phase } from "src/api/migrations/model";
import { getESXHosts } from "src/api/esximigrations/helper";
import MigrationsTable from "./MigrationsTable";

// Register clarity icons
ClarityIcons.addIcons(buildingIcon, clusterIcon, hostIcon, vmIcon);

interface ClusterMigration {
    apiVersion: string;
    kind: string;
    metadata: {
        name: string;
        namespace: string;
        creationTimestamp: string;
        finalizers: string[];
        generation: number;
        resourceVersion: string;
        uid: string;
        ownerReferences: { apiVersion: string; kind: string; name: string; uid: string }[];
    };
    spec: {
        clusterName: string;
        esxiMigrationSequence: string[];
        openstackCredsRef: { name: string };
        rollingMigrationPlanRef: { name: string };
        vmwareCredsRef: { name: string };
    };
    status: {
        currentESXi: string;
        message: string;
        phase: string;
    };
}

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
        width: "1200px",
        maxWidth: "90vw", // For responsiveness on smaller screens
    },
}));

const DrawerContent = styled("div")(({ theme }) => ({
    overflow: "auto",
    padding: theme.spacing(4, 6, 4, 4),
}));

const DrawerHeader = styled("div")(({ theme }) => ({
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    padding: theme.spacing(2, 4),
    borderBottom: `1px solid ${theme.palette.divider}`,
}));

const DrawerFooter = styled("div")(({ theme }) => ({
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    padding: theme.spacing(2, 4),
    borderTop: `1px solid ${theme.palette.divider}`,
}));

function StatusChip({ status }: { status: string }) {
    let color: "success" | "info" | "warning" | "error" | "default" = "default";
    const icon: ReactElement | undefined = undefined;

    switch (status.toLowerCase()) {
        case "migrated":
        case "succeeded":
        case "completed":
        case "done":
            color = "success";
            break;
        case "in progress":
        case "running":
        case "active":
            color = "info";
            break;
        case "pending":
        case "queued":
            color = "warning";
            break;
        case "failed":
            color = "error";
            break;
        case "cordoned":
            color = "warning";
            break;
        default:
            color = "default";
            break;
    }

    return (
        <Chip
            size="small"
            label={status}
            variant="outlined"
            color={color}
            icon={icon}
            sx={{
                borderRadius: '4px',
                height: '24px'
            }}
        />
    );
}

// Status summary component to show counts at the top
const StatusSummary = ({
    items,
    getStatus,
    title
}: {
    items: unknown[];
    getStatus: (item: unknown) => string;
    title: string
}) => {
    const counts = items.reduce<Record<string, number>>((acc, item) => {
        const status = getStatus(item);
        acc[status] = (acc[status] || 0) + 1;
        return acc;
    }, {});

    const total = items.length;
    const completed = counts['Completed'] || counts['Succeeded'] || counts['Done'] || counts['Migrated'] || 0;
    const inProgress = counts['In Progress'] || counts['Running'] || counts['Active'] || 0;
    const pending = counts['Pending'] || counts['Queued'] || 0;
    const failed = counts['Failed'] || 0;
    const cordoned = counts['Cordoned'] || 0;
    const progress = (completed / total) * 100;

    return (
        <Box sx={{ mb: 2 }}>
            <Typography variant="subtitle1" fontWeight="bold">{title}</Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                <Box sx={{ width: '100%', mr: 1 }}>
                    <LinearProgress
                        variant="determinate"
                        value={isNaN(progress) ? 0 : progress}
                        sx={{
                            height: 4,
                            borderRadius: 5,
                            backgroundColor: '#f5f5f5'
                        }}
                    />
                </Box>
                <Box sx={{ minWidth: 35 }}>
                    <Typography variant="body2" color="text.secondary">
                        {completed}/{total}
                    </Typography>
                </Box>
            </Box>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                {completed > 0 && <Chip label={`Completed: ${completed}`} size="small" color="success" variant="outlined" />}
                {inProgress > 0 && <Chip label={`In Progress: ${inProgress}`} size="small" color="info" variant="outlined" />}
                {pending > 0 && <Chip label={`Pending: ${pending}`} size="small" color="warning" variant="outlined" />}
                {failed > 0 && <Chip label={`Failed: ${failed}`} size="small" color="error" variant="outlined" />}
                {cordoned > 0 && <Chip label={`Cordoned: ${cordoned}`} size="small" color="info" variant="outlined" />}
            </Box>
        </Box>
    );
};

function ClusterDetailsDrawer({ open, onClose, clusterMigration, esxHosts, migrations, refetchMigrations }) {
    // ESX Columns for the table
    const esxColumns: GridColDef[] = [
        {
            field: 'name',
            headerName: 'ESX Name',
            flex: 1,
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
            field: 'state',
            headerName: 'State',
            flex: 1,
            renderCell: (params) => <StatusChip status={params.value as string} />
        },
        {
            field: 'ip',
            headerName: 'Current IP',
            flex: 1,
            valueGetter: (value: string) => value || "—"
        },
        {
            field: 'vms',
            headerName: '# VMs',
            flex: 0.5,
            valueGetter: (value: string) => value || "—"
        }
    ];

    // // VM Columns for the table
    // const vmColumns: GridColDef[] = [
    //     {
    //         field: 'name',
    //         headerName: 'VM Name',
    //         flex: 1.5,
    //         renderCell: (params) => (
    //             <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
    //                 <Tooltip title={params.row.powerState === "powered-on" ? "Powered On" : "Powered Off"}>
    //                     <CdsIconWrapper>
    //                         {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
    //                         {/* @ts-ignore */}
    //                         <cds-icon shape="vm" size="md" badge={params.row.powerState === "powered-on" ? "success" : "danger"}></cds-icon>
    //                     </CdsIconWrapper>
    //                 </Tooltip>
    //                 <Box>{params.value}</Box>
    //             </Box>
    //         ),
    //     },
    //     {
    //         field: 'status',
    //         headerName: 'Status',
    //         flex: 0.8,
    //         renderCell: (params) => <StatusChip status={params.value as string} />
    //     },
    //     {
    //         field: 'ip',
    //         headerName: 'Current IP',
    //         flex: 1,
    //         valueGetter: (value: string) => value || "—"
    //     },
    //     {
    //         field: 'networks',
    //         headerName: 'Network Interface(s)',
    //         flex: 1,
    //         renderCell: (params) => {
    //             const networks = (params.row as VM).networks;
    //             return networks ? networks.join(", ") : "—";
    //         }
    //     },
    //     {
    //         field: 'memory',
    //         headerName: 'Memory (MB)',
    //         flex: 0.8,
    //         valueGetter: (value: string) => value || "—"
    //     },
    //     {
    //         field: 'esxHost',
    //         headerName: 'ESX Host',
    //         flex: 1,
    //         valueGetter: (value: string) => value || "—"
    //     },
    // ];

    return (
        <StyledDrawer
            anchor="right"
            open={open}
            onClose={onClose}
        >
            <DrawerHeader>
                <Typography variant="h6">
                    {clusterMigration.spec.clusterName} Details
                </Typography>
                <IconButton onClick={onClose}>
                    <CloseIcon />
                </IconButton>
            </DrawerHeader>
            <DrawerContent>
                <Box sx={{ p: 2 }}>
                    <Box sx={{ mb: 4 }}>
                        <StatusSummary
                            items={esxHosts}
                            getStatus={(esxi) => (esxi as ESXHost).state}
                            title="ESX Migrations"
                        />
                        <Box sx={{ height: 300, width: '100%' }}>
                            <DataGrid
                                rows={esxHosts}
                                columns={esxColumns}
                                disableRowSelectionOnClick
                                initialState={{
                                    pagination: { paginationModel: { pageSize: 10 } },
                                }}
                                pageSizeOptions={[10, 25, 50]}
                                sx={{
                                    '& .MuiDataGrid-columnHeaders': {
                                        backgroundColor: 'rgba(0, 0, 0, 0.04)'
                                    }
                                }}
                            />
                        </Box>
                    </Box>

                    <Box sx={{ mt: 4 }}>
                        <StatusSummary
                            items={migrations}
                            getStatus={(migration) => (migration as Migration).status?.phase}
                            title="VM Migrations"
                        />
                        <Box sx={{ height: 300, width: '100%' }}>
                            <MigrationsTable
                                refetchMigrations={refetchMigrations}
                                migrations={migrations || []}
                            // onDeleteMigration={handleDeleteClick}
                            // onDeleteSelected={handleDeleteSelected}
                            />
                        </Box>
                    </Box>
                </Box>
            </DrawerContent>
            <DrawerFooter>
                <Button variant="outlined" onClick={onClose}>Close</Button>
            </DrawerFooter>
        </StyledDrawer>
    );
}

interface CustomToolbarProps {
    refetchClusterMigrations?: (options?: RefetchOptions) => Promise<QueryObserverResult<ClusterMigration[], Error>>;
    selectedCount: number;
    onDeleteSelected: () => void;
}

const CustomToolbar = ({ refetchClusterMigrations, selectedCount, onDeleteSelected }: CustomToolbarProps) => {
    return (
        <GridToolbarContainer
            sx={{
                p: 2,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center'
            }}
        >
            <div>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="h6" component="h2">
                        Cluster Conversions
                    </Typography>
                </Box>
                {selectedCount > 0 && (
                    <Typography variant="body2" color="text.secondary">
                        {selectedCount} {selectedCount === 1 ? 'row' : 'rows'} selected
                    </Typography>
                )}
            </div>
            <Box sx={{ display: 'flex', gap: 2 }}>
                {selectedCount > 0 && (
                    <Button
                        variant="outlined"
                        color="error"
                        startIcon={<DeleteIcon />}
                        onClick={onDeleteSelected}
                        size="small"
                    >
                        Delete Selected
                    </Button>
                )}
                <CustomSearchToolbar
                    placeholder="Search by Cluster Name, Status, or ESXi Host"
                    onRefresh={refetchClusterMigrations}
                />
            </Box>
        </GridToolbarContainer>
    );
};

interface RollingMigrationsTableProps {
    clusterMigrations: ClusterMigration[];
    esxiMigrations: ESXIMigration[];
    migrations: Migration[];
    refetchClusterMigrations?: (options?: RefetchOptions) => Promise<QueryObserverResult<ClusterMigration[], Error>>;
    refetchMigrations?: (options?: RefetchOptions) => Promise<QueryObserverResult<Migration[], Error>>;
}

export default function RollingMigrationsTable({
    clusterMigrations,
    esxiMigrations,
    migrations,
    refetchClusterMigrations,
    refetchMigrations,
}: RollingMigrationsTableProps) {
    const queryClient = useQueryClient();
    const [selectedCluster, setSelectedCluster] = useState<ClusterMigration | null>(null);
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [selectedRows, setSelectedRows] = useState<GridRowSelectionModel>([]);
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [deleteError, setDeleteError] = useState<string | null>(null);

    const esxHostsByCluster = useMemo(() => {
        const esxisByCluster: Record<string, ESXHost[]> = {};

        clusterMigrations.forEach(cluster => {
            if (cluster.metadata?.name) {
                const clusterName = cluster.spec.clusterName || '';
                esxisByCluster[clusterName] = getESXHosts(esxiMigrations
                    .filter(esxi => esxi.metadata?.labels?.['vjailbreak.k8s.pf9.io/clustermigration']?.toLowerCase().includes(clusterName.toLowerCase()))
                );
            }
        });
        return esxisByCluster;
    }, [clusterMigrations, esxiMigrations]);

    const migrationsByCluster = useMemo(() => {
        const result: Record<string, Migration[]> = {};

        clusterMigrations.forEach(cluster => {
            if (cluster.metadata?.name) {
                const rollingMigrationPlan = cluster.spec.rollingMigrationPlanRef?.name || '';
                result[rollingMigrationPlan] = migrations.filter(migration => migration.metadata?.labels?.['vjailbreak.k8s.pf9.io/rollingmigrationplan']?.includes(rollingMigrationPlan))
            }
        });

        return result;
    }, [clusterMigrations, migrations]);

    const handleOpenDetails = (cluster: ClusterMigration) => {
        setSelectedCluster(cluster);
        setDrawerOpen(true);
    };
    const handleCloseDrawer = () => {
        setDrawerOpen(false);
    };

    const handleDeleteSelected = () => {
        setDeleteDialogOpen(true);
    };

    const handleDeleteClose = () => {
        setDeleteDialogOpen(false);
        setDeleteError(null);
    };


    const handleConfirmDelete = async () => {
        try {
            const selectedClusterMigrations = clusterMigrations.filter(cm =>
                selectedRows.includes(cm.metadata?.name || '')
            );

            await Promise.all(
                selectedClusterMigrations.map(async (migration) => {
                    // const rollingMigrationPlanName = migration.spec.rollingMigrationPlanRef?.name;

                    await deleteClusterMigration(migration.metadata.name);

                })
            );

            queryClient.invalidateQueries({ queryKey: CLUSTER_MIGRATIONS_QUERY_KEY });

            setSelectedRows([]);
        } catch (error) {
            console.error("Failed to delete cluster conversions:", error);
            setDeleteError(error instanceof Error ? error.message : "Failed to delete cluster conversions");
            throw error;
        }
    };

    const handleSelectionChange = (newSelection: GridRowSelectionModel) => {
        setSelectedRows(newSelection);
    };

    // Status ordering for sorting
    const STATUS_ORDER = {
        'Running': 0,
        'In Progress': 0,
        'Active': 0,
        'Failed': 1,
        'Succeeded': 2,
        'Completed': 2,
        'Migrated': 2,
        'Done': 2,
        'Pending': 3,
        'Queued': 3
    };

    // ClusterMigration columns for the main table
    const clusterColumns: GridColDef[] = [
        {
            field: 'clusterName',
            headerName: 'Cluster Name',
            display: 'flex',
            flex: 1,
            renderCell: (params) => (
                <Box sx={{ display: 'flex', alignItems: 'center' }}>
                    <CdsIconWrapper>
                        {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                        {/* @ts-ignore */}
                        <cds-icon shape="cluster" size="md"></cds-icon>
                    </CdsIconWrapper>
                    <Typography variant="body2">{(params.row as ClusterMigration).spec.clusterName || 'Unknown'}</Typography>
                </Box>
            ),
        },
        {
            field: 'status',
            headerName: 'Migration Status',
            flex: 0.5,
            renderCell: (params) => {
                return <StatusChip status={(params.row as ClusterMigration).status?.phase || 'Unknown'} />;
            },
            sortComparator: (v1, v2) => {
                const order1 = STATUS_ORDER[v1] ?? Number.MAX_SAFE_INTEGER;
                const order2 = STATUS_ORDER[v2] ?? Number.MAX_SAFE_INTEGER;
                return order1 - order2;
            }
        },
        {
            field: 'esxCount',
            headerName: 'ESX Hosts',
            flex: 0.5,
            renderCell: (params) => {
                const clusterName = (params.row as ClusterMigration).spec?.clusterName || '';
                return esxHostsByCluster[clusterName]?.length || 0;
            },
        },
        {
            field: 'vmCount',
            headerName: 'VMs',
            flex: 0.5,
            renderCell: (params) => {
                const rollingMigrationPlan = (params.row as ClusterMigration).spec?.rollingMigrationPlanRef?.name || '';
                return migrationsByCluster[rollingMigrationPlan]?.length || 0;
            },
        },
        {
            field: 'progress',
            headerName: 'Migration Progress',
            flex: 1,
            renderCell: (params) => {
                const clusterName = (params.row as ClusterMigration).spec?.clusterName || '';
                const rollingMigrationPlan = (params.row as ClusterMigration).spec?.rollingMigrationPlanRef?.name || '';

                // ESX Hosts progress
                const esxHosts = esxHostsByCluster[clusterName] || [];
                const totalEsx = esxHosts.length;
                const migratedEsx = esxHosts.filter(host => host.state === Phase.Succeeded).length;
                const esxProgress = totalEsx > 0 ? (migratedEsx / totalEsx) * 100 : 0;

                // VMs progress
                const migrations = migrationsByCluster[rollingMigrationPlan] || [];
                const totalVms = migrations.length;
                const migratedVms = migrations.filter(migration => migration.status?.phase === Phase.Succeeded).length;
                const vmProgress = totalVms > 0 ? (migratedVms / totalVms) * 100 : 0;

                return (
                    <Box sx={{ width: '100%' }}>
                        {/* ESX Hosts progress - single line layout */}
                        <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
                            <Typography variant="caption" color="text.secondary" sx={{ width: 70 }}>
                                ESX Hosts:
                            </Typography>
                            <Box sx={{ flex: 1, mx: 1, display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                                <LinearProgress
                                    variant="determinate"
                                    value={esxProgress}
                                    sx={{
                                        width: 150,
                                        borderRadius: 1,
                                        backgroundColor: 'rgba(0, 0, 0, 0.08)'
                                    }}
                                />
                                <Typography variant="caption" sx={{ width: 35, textAlign: 'right' }}>
                                    {migratedEsx}/{totalEsx}
                                </Typography>
                            </Box>
                        </Box>

                        {/* VMs progress - single line layout */}
                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                            <Typography variant="caption" color="text.secondary" sx={{ width: 70 }}>
                                VMs:
                            </Typography>
                            <Box sx={{ flex: 1, mx: 1, display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                                <LinearProgress
                                    variant="determinate"
                                    value={vmProgress}
                                    sx={{
                                        width: 150,
                                        borderRadius: 1,
                                        backgroundColor: 'rgba(0, 0, 0, 0.08)'
                                    }}
                                />
                                <Typography variant="caption" sx={{ width: 35, textAlign: 'right' }}>
                                    {migratedVms}/{totalVms}
                                </Typography>
                            </Box>
                        </Box>
                    </Box>
                );
            }
        },
        {
            field: 'actions',
            headerName: 'Actions',
            flex: 1,
            renderCell: (params) => (
                <Button
                    variant="text"
                    size="small"
                    startIcon={<VisibilityIcon />}
                    onClick={() => handleOpenDetails(params.row as ClusterMigration)}
                >
                    Details
                </Button>
            ),
        },
    ];

    return (
        <Box sx={{ width: '100%', height: '100%' }}>
            <DataGrid
                rows={clusterMigrations}
                columns={clusterColumns}
                getRowId={(row: ClusterMigration) => row.metadata?.name || ''}
                initialState={{
                    pagination: { paginationModel: { pageSize: 25 } },
                    sorting: {
                        sortModel: [{ field: 'status', sort: 'asc' }],
                    },
                }}
                pageSizeOptions={[5, 10, 25]}
                localeText={{ noRowsLabel: "No Migrations Available" }}
                slots={{
                    toolbar: () => (
                        <CustomToolbar
                            refetchClusterMigrations={refetchClusterMigrations}
                            selectedCount={selectedRows.length}
                            onDeleteSelected={handleDeleteSelected}
                        />
                    ),
                }}
                checkboxSelection
                onRowSelectionModelChange={handleSelectionChange}
                rowSelectionModel={selectedRows}
                disableRowSelectionOnClick
            />

            {selectedCluster && (
                <ClusterDetailsDrawer
                    open={drawerOpen}
                    onClose={handleCloseDrawer}
                    clusterMigration={selectedCluster}
                    esxHosts={esxHostsByCluster[selectedCluster.spec.clusterName || ''] || []}
                    migrations={migrationsByCluster[selectedCluster.spec.rollingMigrationPlanRef?.name || ''] || []}
                    refetchMigrations={refetchMigrations}
                // refetchESXMigrations={refetchESXMigrations}
                />
            )}

            <ConfirmationDialog
                open={deleteDialogOpen}
                onClose={handleDeleteClose}
                title="Confirm Delete"
                icon={<WarningIcon color="warning" />}
                message={selectedRows.length > 1
                    ? "Are you sure you want to delete these cluster conversions?"
                    : `Are you sure you want to delete the selected cluster conversion?`
                }
                items={clusterMigrations
                    .filter(cm => selectedRows.includes(cm.metadata?.name || ''))
                    .map(cm => ({
                        id: cm.metadata?.name || '',
                        name: cm.spec?.clusterName || cm.metadata?.name || ''
                    }))}
                actionLabel="Delete"
                actionColor="error"
                actionVariant="outlined"
                onConfirm={handleConfirmDelete}
                errorMessage={deleteError}
                onErrorChange={setDeleteError}
            />
        </Box>
    );
}
