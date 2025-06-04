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
        valueGetter: (_, row) => row.metadata?.name,
        flex: 1.5,
    },
    {
        field: "status",
        headerName: "Status",
        valueGetter: (_, row) => row?.status?.phase || "Pending",
        flex: 1,
        sortComparator: (v1, v2) => {
            const order1 = STATUS_ORDER[v1] ?? Number.MAX_SAFE_INTEGER;
            const order2 = STATUS_ORDER[v2] ?? Number.MAX_SAFE_INTEGER;
            return order1 - order2;
        }
    },
    {
        field: "status.conditions",
        headerName: "Progress",
        valueGetter: (_, row) => getProgressText(row.status?.phase, row.status?.conditions),
        flex: 3,
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
    onDeleteMigration: (name: string) => void;
    onDeleteSelected: (migrations: Migration[]) => void;
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
            columns={columns}
            initialState={{
                pagination: { paginationModel: { page: 0, pageSize: 25 } },
                sorting: {
                    sortModel: [{ field: 'status', sort: 'asc' }],
                },
            }}
            pageSizeOptions={[25, 50, 100]}
            localeText={{ noRowsLabel: "No Migrations Available" }}
            getRowId={(row) => row.metadata?.name}
            checkboxSelection
            onRowSelectionModelChange={handleSelectionChange}
            rowSelectionModel={selectedRows}
            slots={{
                toolbar: () => (
                    <CustomToolbar
                        numSelected={selectedRows.length}
                        onDeleteSelected={() => {
                            const selectedMigrations = migrations?.filter(
                                m => selectedRows.includes(m.metadata?.name)
                            );
                            onDeleteSelected(selectedMigrations || []);
                        }}
                        refetchMigrations={refetchMigrations}
                    />
                ),
            }}
        />
    );
} 