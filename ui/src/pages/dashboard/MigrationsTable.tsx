import { DataGrid, GridColDef, GridRowSelectionModel, GridToolbarContainer } from "@mui/x-data-grid";
import { Button, Typography, Box, IconButton, Tooltip } from "@mui/material";
import DeleteIcon from '@mui/icons-material/DeleteOutlined';
import MigrationIcon from '@mui/icons-material/SwapHoriz';
import ReplayIcon from '@mui/icons-material/Replay';
import ListAltIcon from '@mui/icons-material/ListAlt';
import { useState } from "react";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import LogsDrawer from "src/components/LogsDrawer";
import { Condition, Migration, Phase } from "src/api/migrations/model";
import MigrationProgress from "./MigrationProgress";
import { QueryObserverResult } from "@tanstack/react-query";
import { RefetchOptions } from "@tanstack/react-query";
import { calculateTimeElapsed } from "src/utils";
import { TriggerAdminCutoverButton } from "src/components/TriggerAdminCutover/TriggerAdminCutoverButton";
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { triggerAdminCutover, deleteMigration } from "src/api/migrations/migrations";
import ConfirmationDialog from "src/components/dialogs/ConfirmationDialog";

// Move the STATUS_ORDER and columns from Dashboard.tsx to here
const STATUS_ORDER = {
    'Running': 0,
    'Failed': 1,
    'Succeeded': 2,
    'Pending': 3
}
const PHASE_STEPS = {
    [Phase.Pending]: 1,
    [Phase.Validating]: 2,
    [Phase.AwaitingDataCopyStart]: 3,
    [Phase.CopyingBlocks]: 4,
    [Phase.CopyingChangedBlocks]: 5,
    [Phase.ConvertingDisk]: 6,
    [Phase.AwaitingCutOverStartTime]: 7,
    [Phase.AwaitingAdminCutOver]: 8,
    [Phase.Succeeded]: 9,
    [Phase.Failed]: 9,
}

const getProgressText = (phase: Phase | undefined, conditions: Condition[] | undefined) => {
    if (!phase || phase === Phase.Unknown) {
        return "Unknown Status";
    }

    const stepNumber = PHASE_STEPS[phase] || 0;
    const totalSteps = 9;

    // Get the most recent condition's message
    const latestCondition = conditions?.sort((a, b) =>
        new Date(b.lastTransitionTime).getTime() - new Date(a.lastTransitionTime).getTime()
    )[0];

    const message = latestCondition?.message || phase;

    if (phase === Phase.Failed || phase === Phase.Succeeded) {
        return `${phase} - ${message}`;
    }

    return `STEP ${stepNumber}/${totalSteps}: ${phase} - ${message}`;
}

const columns: GridColDef[] = [
    {
        field: "name",
        headerName: "Name",
        valueGetter: (_, row) => row.spec?.vmName,
        flex: 0.7,
    },
    {
        field: "status",
        headerName: "Status",
        valueGetter: (_, row) => row?.status?.phase || "Pending",
        flex: 0.5,
        sortComparator: (v1, v2) => {
            const order1 = STATUS_ORDER[v1] ?? Number.MAX_SAFE_INTEGER;
            const order2 = STATUS_ORDER[v2] ?? Number.MAX_SAFE_INTEGER;
            return order1 - order2;
        }
    },
    {
        field: "agent",
        headerName: "Agent",
        valueGetter: (_, row) => row.status?.agentName,
        flex: 1,
    },
    {
        field: "timeElapsed",
        headerName: "Time Elapsed",
        valueGetter: (_, row) => calculateTimeElapsed(row.metadata?.creationTimestamp, row.status),
        flex: 0.8,
    },
    {
        field: "createdAt",
        headerName: "Created At",
        valueGetter: (_, row) => {
            if (row.metadata?.creationTimestamp) {
                return new Date(row.metadata.creationTimestamp).toLocaleString();
            }
            return '-';
        },
        flex: 1,
    },
    {
        field: "status.conditions",
        headerName: "Progress",
        valueGetter: (_, row) => getProgressText(row.status?.phase, row.status?.conditions),
        flex: 2,
        renderCell: (params) => {
            const phase = params.row?.status?.phase
            const conditions = params.row?.status?.conditions
            return conditions ? (
                <MigrationProgress
                    phase={phase}
                    progressText={getProgressText(phase, conditions)}
                />
            ) : null
        },
    },
    {
        field: "actions",
        headerName: "Actions",
        flex: 1,
        renderCell: (params) => {
            const phase = params.row?.status?.phase;
            const initiateCutover = params.row?.spec?.initiateCutover;
            const migrationName = params.row?.metadata?.name;
            const namespace = params.row?.metadata?.namespace;
            const showRetryButton = phase === Phase.Failed;

            const handleRetry = async () => {
                if (!migrationName || !namespace) {
                    console.error("Cannot retry: migration name or namespace is missing.");
                    return;
                }
                try {
                    await deleteMigration(migrationName, namespace);
                    params.row.refetchMigrations?.();
                } catch (error) {
                    console.error(`Failed to delete migration '${migrationName}' for retry:`, error);
                }
            };
            
            // Show admin cutover button if:
            // 1. initiateCutover is false (manual cutover)
            // 2. Phase is AwaitingAdminCutOver

            const showAdminCutover = initiateCutover && (phase === Phase.AwaitingAdminCutOver);

            return (
                <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                    {params.row.spec?.podRef && (
                        <Tooltip title="View pod logs">
                            <IconButton
                                onClick={(e) => {
                                    e.stopPropagation();
                                    params.row.setSelectedPod({
                                        name: params.row.spec.podRef,
                                        namespace: params.row.metadata?.namespace || '',
                                        migrationName: params.row.metadata?.name || ''
                                    });
                                    params.row.setLogsDrawerOpen(true);
                                }}
                                size="small"
                                sx={{
                                    cursor: 'pointer',
                                    position: 'relative'
                                }}
                            >
                                <ListAltIcon />
                            </IconButton>
                        </Tooltip>
                    )}

                    {showAdminCutover && (
                        <TriggerAdminCutoverButton
                            migrationName={migrationName}
                            onSuccess={() => {
                                params.row.refetchMigrations?.();
                            }}
                            onError={(error) => {
                                console.error("Failed to trigger cutover:", error);
                            }}
                        />
                    )}

                    {showRetryButton && (
                        <Tooltip title="Retry migration">
                            <IconButton
                                onClick={(e) => {
                                    e.stopPropagation();
                                    handleRetry();
                                }}
                                size="small"
                                sx={{
                                cursor: 'pointer',
                                position: 'relative'
                            }}
                            >
                                <ReplayIcon />
                            </IconButton>
                        </Tooltip>
                    )}
                    
                    <Tooltip title={"Delete migration"}>
                        <IconButton
                            onClick={(e) => {
                                e.stopPropagation();
                                params.row.onDelete(params.row.metadata?.name);
                            }}
                            size="small"
                            sx={{
                                cursor: 'pointer',
                                position: 'relative'
                            }}
                        >
                            <DeleteIcon />
                        </IconButton>
                    </Tooltip>
                </Box>
            );
        },
    },
]

interface CustomToolbarProps {
    numSelected: number;
    onDeleteSelected: () => void;
    onBulkAdminCutover: () => void;
    numEligibleForCutover: number;
    refetchMigrations: (options?: RefetchOptions) => Promise<QueryObserverResult<Migration[], Error>>;
}


const CustomToolbar = ({ numSelected, onDeleteSelected, onBulkAdminCutover, numEligibleForCutover, refetchMigrations }: CustomToolbarProps) => {
    return (
        <GridToolbarContainer
            sx={{
                p: 2,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center'
            }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <MigrationIcon />
                <Typography variant="h6" component="h2">
                    Migrations
                </Typography>
            </Box>
            <Box sx={{ display: 'flex', gap: 2 }}>
                {numSelected > 0 && (
                    <>
                        <Button
                            variant="outlined"
                            color="error"
                            startIcon={<DeleteIcon />}
                            onClick={onDeleteSelected}
                            sx={{ height: 40 }}
                        >
                            Delete Selected ({numSelected})
                        </Button>
                        
                        {numEligibleForCutover > 0 && (
                            <Button
                                variant="outlined"
                                color="primary"
                                startIcon={<PlayArrowIcon />}
                                onClick={onBulkAdminCutover}
                                sx={{ height: 40 }}
                            >
                                Trigger Cutover ({numEligibleForCutover})
                            </Button>
                        )}
                    </>
                )}
                <CustomSearchToolbar
                    placeholder="Search by Name, Status, or Progress"
                    onRefresh={refetchMigrations}
                />
            </Box>
        </GridToolbarContainer>
    );
};

interface MigrationsTableProps {
    migrations: Migration[];
    onDeleteMigration?: (name: string) => void;
    onDeleteSelected?: (migrations: Migration[]) => void;
    refetchMigrations: (options?: RefetchOptions) => Promise<QueryObserverResult<Migration[], Error>>;
}

export default function MigrationsTable({
    migrations,
    onDeleteMigration,
    onDeleteSelected,
    refetchMigrations
}: MigrationsTableProps) {
    const [selectedRows, setSelectedRows] = useState<GridRowSelectionModel>([]);
    const [isBulkCutoverLoading, setIsBulkCutoverLoading] = useState(false);
    const [bulkCutoverDialogOpen, setBulkCutoverDialogOpen] = useState(false);
    const [bulkCutoverError, setBulkCutoverError] = useState<string | null>(null);
    const [logsDrawerOpen, setLogsDrawerOpen] = useState(false);
    const [selectedPod, setSelectedPod] = useState<{ name: string; namespace: string; migrationName?: string } | null>(null);

    const handleSelectionChange = (newSelection: GridRowSelectionModel) => {
        setSelectedRows(newSelection);
    };

    // Get selected migrations that are eligible for admin cutover
    const selectedMigrations = migrations?.filter(m => selectedRows.includes(m.metadata?.name)) || [];
    const eligibleForCutover = selectedMigrations.filter(migration => 
    migration.status?.phase === Phase.AwaitingAdminCutOver
    );

    const handleBulkAdminCutover = async () => {
        if (eligibleForCutover.length === 0) return;
        
        setBulkCutoverError(null);
        setIsBulkCutoverLoading(true);
        
        try {
            await Promise.all(
                eligibleForCutover.map(async (migration) => {
                    const result = await triggerAdminCutover("migration-system", migration.metadata?.name || "");
                    if (!result.success) {
                        throw new Error(result.message);
                    }
                    return result;
                })
            );
            
            // Refresh the migrations table
            await refetchMigrations();
            
            // Clear selection after successful cutover
            setSelectedRows([]);
            // Don't close here - let ConfirmationDialog handle it
        } catch (error) {
            console.error("Failed to trigger bulk admin cutover:", error);
            const errorMessage = error instanceof Error ? error.message : "Failed to trigger bulk admin cutover";
            setBulkCutoverError(errorMessage);
            // Re-throw to prevent ConfirmationDialog from auto-closing
            throw error;
        } finally {
            setIsBulkCutoverLoading(false);
        }
    };

    const handleCloseBulkCutoverDialog = () => {
        if (!isBulkCutoverLoading) {
            setBulkCutoverDialogOpen(false);
            setBulkCutoverError(null);
        }
    };

    const migrationsWithActions = migrations?.map(migration => ({
        ...migration,
        onDelete: onDeleteMigration,
        refetchMigrations,
        setSelectedPod,
        setLogsDrawerOpen
    })) || [];

    return (
        <>
            <DataGrid
                rows={migrationsWithActions}
                columns={onDeleteSelected === undefined && onDeleteMigration === undefined ? columns.filter(column => column.field !== "actions") : columns}
                initialState={{
                    pagination: { paginationModel: { page: 0, pageSize: 25 } },
                    sorting: {
                        sortModel: [{ field: 'status', sort: 'asc' }],
                    },
                }}
                pageSizeOptions={[25, 50, 100]}
                localeText={{ noRowsLabel: "No Migrations Available" }}
                getRowId={(row) => row.metadata?.name}
                checkboxSelection={onDeleteSelected !== undefined && onDeleteMigration !== undefined}
                onRowSelectionModelChange={handleSelectionChange}
                rowSelectionModel={selectedRows}
                disableRowSelectionOnClick
                loading={isBulkCutoverLoading}
                slots={{
                    toolbar: onDeleteSelected !== undefined && onDeleteMigration !== undefined ? () => (
                        <CustomToolbar
                            numSelected={selectedRows.length}
                            numEligibleForCutover={eligibleForCutover.length}
                            onDeleteSelected={() => {
                                const selectedMigrations = migrations?.filter(
                                    m => selectedRows.includes(m.metadata?.name)
                                );
                                if (onDeleteSelected) {
                                    onDeleteSelected(selectedMigrations || []);
                                }
                            }}
                            onBulkAdminCutover={() => setBulkCutoverDialogOpen(true)}
                            refetchMigrations={refetchMigrations}
                        />
                    ) : undefined,
                }}
            />

            <ConfirmationDialog
                open={bulkCutoverDialogOpen}
                onClose={handleCloseBulkCutoverDialog}
                title="Confirm Admin Cutover"
                icon={<PlayArrowIcon color="primary" />}
                message={
                    eligibleForCutover.length > 1
                        ? `Are you sure you want to trigger admin cutover for these ${eligibleForCutover.length} migrations?\n\n${eligibleForCutover.map(m => `• ${m.metadata?.name}`).join('\n')}\n\nThis will start the cutover process and cannot be undone.`
                        : `Are you sure you want to trigger admin cutover for migration "${eligibleForCutover[0]?.metadata?.name}"?\n\nThis will start the cutover process and cannot be undone.`
                }
                items={eligibleForCutover.map(migration => ({
                    id: migration.metadata?.name || '',
                    name: migration.metadata?.name || ''
                }))}
                actionLabel="Trigger Cutover"
                actionColor="primary"
                actionVariant="contained"
                onConfirm={handleBulkAdminCutover}
                errorMessage={bulkCutoverError}
                onErrorChange={setBulkCutoverError}
            />

            <LogsDrawer
                open={logsDrawerOpen}
                onClose={() => setLogsDrawerOpen(false)}
                podName={selectedPod?.name || ''}
                namespace={selectedPod?.namespace || ''}
                migrationName={selectedPod?.migrationName || ''}
            />
        </>
    );
}