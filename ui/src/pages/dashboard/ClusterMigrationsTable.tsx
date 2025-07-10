import {
    DataGrid,
    GridColDef,
    GridRowSelectionModel,
    GridToolbarContainer
} from "@mui/x-data-grid";
import {
    Button,
    Typography,
    Box,
    IconButton,
    Tooltip,
    Chip
} from "@mui/material";
import DeleteIcon from '@mui/icons-material/DeleteOutlined';
import ClusterIcon from '@mui/icons-material/Hub';
import { useState } from "react";
import CustomSearchToolbar from "src/components/grid/CustomSearchToolbar";
import { ClusterMigration } from "src/api/clustermigrations/model";
import { QueryObserverResult } from "@tanstack/react-query";
import { RefetchOptions } from "@tanstack/react-query";
import { getClusterMigrationPhase, getClusterName, getCurrentESXi } from "src/api/clustermigrations/helper";

// Phase ordering for sorting
const STATUS_ORDER = {
    'Running': 0,
    'Failed': 1,
    'Succeeded': 2,
    'Pending': 3
}

const getStatusColor = (status: string) => {
    switch (status) {
        case 'Succeeded':
            return 'success';
        case 'Failed':
            return 'error';
        case 'Pending':
            return 'warning';
        default:
            return 'primary';
    }
};

const columns: GridColDef[] = [
    {
        field: "name",
        headerName: "Name",
        valueGetter: (_, row) => row.metadata?.name,
        flex: 1.5,
    },
    {
        field: "clusterName",
        headerName: "Cluster",
        valueGetter: (_, row) => getClusterName(row),
        flex: 1.5,
    },
    {
        field: "currentESXi",
        headerName: "Current ESXi",
        valueGetter: (_, row) => getCurrentESXi(row),
        flex: 1.5,
    },
    {
        field: "status",
        headerName: "Status",
        valueGetter: (_, row) => getClusterMigrationPhase(row),
        flex: 1,
        renderCell: (params) => {
            const status = params.value;
            return (
                <Chip
                    label={status}
                    color={getStatusColor(status)}
                    size="small"
                    variant="outlined"
                />
            );
        },
        sortComparator: (v1, v2) => {
            const order1 = STATUS_ORDER[v1] ?? Number.MAX_SAFE_INTEGER;
            const order2 = STATUS_ORDER[v2] ?? Number.MAX_SAFE_INTEGER;
            return order1 - order2;
        }
    },
    {
        field: "actions",
        headerName: "Actions",
        flex: 1,
        renderCell: (params) => {
            const phase = getClusterMigrationPhase(params.row);
            const isDisabled = !phase || phase === "Running" || phase === "Pending";

            return (
                <Tooltip title={isDisabled ? "Cannot delete while conversion is in progress" : "Delete cluster conversion"} >
                    <IconButton
                        onClick={(e) => {
                            e.stopPropagation();
                            params.row.onDelete(params.row.metadata?.name);
                        }}
                        disabled={isDisabled}
                        size="small"
                        sx={{
                            cursor: isDisabled ? 'not-allowed' : 'pointer',
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
    refetchClusterMigrations: (options?: RefetchOptions) => Promise<QueryObserverResult<ClusterMigration[], Error>>;
}

const CustomToolbar = ({ numSelected, onDeleteSelected, refetchClusterMigrations }: CustomToolbarProps) => {
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
                <ClusterIcon />
                <Typography variant="h6" component="h2">
                    Cluster Conversions
                </Typography>
            </Box>
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
                    placeholder="Search by Name, Cluster, or ESXi"
                    onRefresh={refetchClusterMigrations}
                />
            </Box>
        </GridToolbarContainer>
    );
};

interface ClusterMigrationsTableProps {
    clusterMigrations: ClusterMigration[];
    onDeleteClusterMigration: (name: string) => void;
    onDeleteSelected: (clusterMigrations: ClusterMigration[]) => void;
    refetchClusterMigrations: (options?: RefetchOptions) => Promise<QueryObserverResult<ClusterMigration[], Error>>;
}

export default function ClusterMigrationsTable({
    clusterMigrations,
    onDeleteClusterMigration,
    onDeleteSelected,
    refetchClusterMigrations
}: ClusterMigrationsTableProps) {
    const [selectedRows, setSelectedRows] = useState<GridRowSelectionModel>([]);

    const handleSelectionChange = (newSelection: GridRowSelectionModel) => {
        setSelectedRows(newSelection);
    };

    const clusterMigrationsWithActions = clusterMigrations?.map(clusterMigration => ({
        ...clusterMigration,
        onDelete: onDeleteClusterMigration
    })) || [];

    const isRowSelectable = (params) => {
        const phase = getClusterMigrationPhase(params.row);
        return !(!phase || phase === "Running" || phase === "Pending");
    };

    return (
        <DataGrid
            rows={clusterMigrationsWithActions}
            columns={columns}
            initialState={{
                pagination: { paginationModel: { page: 0, pageSize: 25 } },
                sorting: {
                    sortModel: [{ field: 'status', sort: 'asc' }],
                },
            }}
            pageSizeOptions={[25, 50, 100]}
            localeText={{ noRowsLabel: "No Cluster Conversions Available" }}
            getRowId={(row) => row.metadata?.name}
            checkboxSelection
            isRowSelectable={isRowSelectable}
            onRowSelectionModelChange={handleSelectionChange}
            rowSelectionModel={selectedRows}
            slots={{
                toolbar: () => (
                    <CustomToolbar
                        numSelected={selectedRows.length}
                        onDeleteSelected={() => {
                            const selectedClusterMigrations = clusterMigrations?.filter(
                                m => selectedRows.includes(m.metadata?.name)
                            );
                            onDeleteSelected(selectedClusterMigrations || []);
                        }}
                        refetchClusterMigrations={refetchClusterMigrations}
                    />
                ),
            }}
        />
    );
} 