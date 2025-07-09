import { DataGrid, GridColDef, GridRowSelectionModel, GridToolbarContainer } from "@mui/x-data-grid";
import { Button, Typography, Box, IconButton, Tooltip } from "@mui/material";
import DeleteIcon from '@mui/icons-material/DeleteOutlined';
import { useState } from "react";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import { Condition, Migration, Phase } from "src/api/migrations/model";
import MigrationProgress from "./MigrationProgress";
import { QueryObserverResult } from "@tanstack/react-query";
import { RefetchOptions } from "@tanstack/react-query";

// Move the STATUS_ORDER and columns from Dashboard.tsx to here
const STATUS_ORDER = {
    'Running': 0,
    'Failed': 1,
    'Blocked': 2,
    'Succeeded': 3,
    'Pending': 4
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

    const latestCondition = conditions?.sort((a, b) =>
        new Date(b.lastTransitionTime).getTime() - new Date(a.lastTransitionTime).getTime()
    )[0];

    const message = latestCondition?.message || phase;
    const reason = latestCondition?.reason || '';
    const isBlocked = reason?.toString() === 'Blocked' || phase === Phase.Blocked;

    if (isBlocked) {
        if (message.startsWith('CONFLICT:')) {
            const parts = message.split(':');
            
            if (parts.length >= 3) {
                const conflictType = parts[1];
                const details = parts.slice(2).join(':');

                switch(conflictType) {
                    case 'MAC_ALREADY_ALLOCATED':
                        return `Blocked: MAC address conflict - ${details}`;
                    case 'IP_ALREADY_ALLOCATED':
                        return `Blocked: IP address conflict - ${details}`;
                    case 'IP_NOT_IN_ALLOCATION_POOL':
                        return `Blocked: IP not in allocation pool - ${details}`;
                    case 'NETWORK_NOT_MAPPED':
                        return `Blocked: Configuration error - ${details}`;
                    default:
                        return `Blocked: ${conflictType} - ${details}`;
                }
            }
        }
        
        return `Blocked: ${message}`;
    }

    const stepNumber = PHASE_STEPS[phase] || 0;
    const totalSteps = 9;

    if (phase === Phase.Failed || phase === Phase.Succeeded) {
        return `${phase} - ${message}`;
    }

    return `STEP ${stepNumber}/${totalSteps}: ${phase} - ${message}`;
};

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
        valueGetter: (_, row) => {
            const phase: Phase = row?.status?.phase || Phase.Pending;
            const condition = row?.status?.conditions?.[0];
            // Use the condition reason if it's a blocked status, otherwise use the phase
            return condition?.reason === 'Blocked' ? Phase.Blocked : phase;
        },
        flex: 0.5,
        sortComparator: (v1: Phase, v2: Phase) => {
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
        field: "status.conditions",
        headerName: "Progress",
        valueGetter: (_, row) => getProgressText(row.status?.phase, row.status?.conditions),
        flex: 2,
        renderCell: (params) => {
            const conditions = params.row?.status?.conditions
            const latestCondition = conditions?.sort((a, b) => new Date(b.lastTransitionTime).getTime() - new Date(a.lastTransitionTime).getTime())[0];
            let phase = params.row?.status?.phase;
            if (latestCondition?.reason === 'Blocked') {
                phase = Phase.Blocked;
            }
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

            return (
                <Tooltip title={"Delete migration"} >
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
            );
        },
    },
]

interface CustomToolbarProps {
    numSelected: number;
    onDeleteSelected: () => void;
    refetchMigrations: (options?: RefetchOptions) => Promise<QueryObserverResult<Migration[], Error>>;
}


const CustomToolbar = ({ numSelected, onDeleteSelected, refetchMigrations }: CustomToolbarProps) => {
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
                <Typography variant="h6" component="h2">
                    Migrations
                </Typography>
            </div>
            <Box sx={{ display: 'flex', gap: 2 }}>
                {numSelected > 0 && (
                    <Button
                        variant="outlined"
                        color="error"
                        startIcon={<DeleteIcon />}
                        onClick={onDeleteSelected}
                        sx={{ height: 40 }}
                    >
                        Delete Selected ({numSelected})
                    </Button>
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

    const handleSelectionChange = (newSelection: GridRowSelectionModel) => {
        setSelectedRows(newSelection);
    };

    const migrationsWithActions = migrations?.map(migration => ({
        ...migration,
        onDelete: onDeleteMigration
    })) || [];

    return (
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
            slots={{
                toolbar: onDeleteSelected !== undefined && onDeleteMigration !== undefined ? () => (
                    <CustomToolbar
                        numSelected={selectedRows.length}
                        onDeleteSelected={() => {
                            const selectedMigrations = migrations?.filter(
                                m => selectedRows.includes(m.metadata?.name)
                            );
                            if (onDeleteSelected) {
                                onDeleteSelected(selectedMigrations || []);
                            }
                        }}
                        refetchMigrations={refetchMigrations}
                    />
                ) : undefined,
            }}
        />
    );
} 