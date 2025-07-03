import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Grid,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Toolbar,
  Typography,
  Checkbox,
  Menu,
  MenuItem,
  Alert,
  Snackbar,
  CircularProgress,
  Tooltip,
  FormControl,
  InputLabel,
  Select,
  SelectChangeEvent,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Visibility as ViewIcon,
  MoreVert as MoreVertIcon,
  Refresh as RefreshIcon,
  Download as DownloadIcon,
  Search as SearchIcon,
  FilterList as FilterIcon,
} from '@mui/icons-material';
import { useTheme } from '@mui/material/styles';
import { SharedRDM, SharedRDMSpec, SharedRDMStatus } from '../../api/shared-rdm/types';
import {
  getSharedRDMs,
  createSharedRDM,
  updateSharedRDM,
  deleteSharedRDM,
  getSharedRDMStatus,
} from '../../api/shared-rdm/api';
import { SharedRDMForm } from '../../components/forms/SharedRDMForm';

interface SharedRDMPageState {
  sharedRDMs: SharedRDM[];
  loading: boolean;
  error: string | null;
  selectedItems: string[];
  searchTerm: string;
  statusFilter: string;
  page: number;
  rowsPerPage: number;
  formOpen: boolean;
  detailsOpen: boolean;
  editingItem: SharedRDM | null;
  viewingItem: SharedRDM | null;
  anchorEl: HTMLElement | null;
  actionMenuId: string | null;
  snackbarOpen: boolean;
  snackbarMessage: string;
  snackbarSeverity: 'success' | 'error' | 'warning' | 'info';
}

const POLLING_INTERVAL = 5000; // 5 seconds

const getStatusColor = (phase: string): 'default' | 'primary' | 'secondary' | 'error' | 'info' | 'success' | 'warning' => {
  switch (phase?.toLowerCase()) {
    case 'ready':
      return 'success';
    case 'creating':
    case 'discovering':
      return 'info';
    case 'failed':
      return 'error';
    default:
      return 'default';
  }
};

const formatBytes = (bytes: number): string => {
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  if (bytes === 0) return '0 Bytes';
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round(bytes / Math.pow(1024, i) * 100) / 100 + ' ' + sizes[i];
};

export const SharedRDMPage: React.FC = () => {
  const theme = useTheme();
  const [state, setState] = useState<SharedRDMPageState>({
    sharedRDMs: [],
    loading: true,
    error: null,
    selectedItems: [],
    searchTerm: '',
    statusFilter: '',
    page: 0,
    rowsPerPage: 10,
    formOpen: false,
    detailsOpen: false,
    editingItem: null,
    viewingItem: null,
    anchorEl: null,
    actionMenuId: null,
    snackbarOpen: false,
    snackbarMessage: '',
    snackbarSeverity: 'info',
  });

  const showSnackbar = useCallback((message: string, severity: 'success' | 'error' | 'warning' | 'info' = 'info') => {
    setState(prev => ({
      ...prev,
      snackbarOpen: true,
      snackbarMessage: message,
      snackbarSeverity: severity,
    }));
  }, []);

  const loadSharedRDMs = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, loading: true, error: null }));
      const data = await getSharedRDMs();
      setState(prev => ({ ...prev, sharedRDMs: data, loading: false }));
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to load SharedRDM resources';
      setState(prev => ({ ...prev, error: errorMessage, loading: false }));
      showSnackbar(errorMessage, 'error');
    }
  }, [showSnackbar]);

  const pollStatus = useCallback(async () => {
    if (state.sharedRDMs.length === 0) return;
    
    try {
      const updatedRDMs = await Promise.all(
        state.sharedRDMs.map(async (rdm) => {
          try {
            const status = await getSharedRDMStatus(rdm.metadata.name);
            return { ...rdm, status };
          } catch {
            return rdm; // Keep existing data if status fetch fails
          }
        })
      );
      setState(prev => ({ ...prev, sharedRDMs: updatedRDMs }));
    } catch (error) {
      console.error('Failed to poll status:', error);
    }
  }, [state.sharedRDMs]);

  useEffect(() => {
    loadSharedRDMs();
  }, [loadSharedRDMs]);

  useEffect(() => {
    const interval = setInterval(pollStatus, POLLING_INTERVAL);
    return () => clearInterval(interval);
  }, [pollStatus]);

  const handleCreate = () => {
    setState(prev => ({ ...prev, formOpen: true, editingItem: null }));
  };

  const handleEdit = (item: SharedRDM) => {
    setState(prev => ({ ...prev, formOpen: true, editingItem: item }));
  };

  const handleView = (item: SharedRDM) => {
    setState(prev => ({ ...prev, detailsOpen: true, viewingItem: item }));
  };

  const handleDelete = async (item: SharedRDM) => {
    try {
      await deleteSharedRDM(item.metadata.name);
      showSnackbar(`SharedRDM "${item.metadata.name}" deleted successfully`, 'success');
      loadSharedRDMs();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to delete SharedRDM';
      showSnackbar(errorMessage, 'error');
    }
  };

  const handleBulkDelete = async () => {
    try {
      await Promise.all(state.selectedItems.map(name => deleteSharedRDM(name)));
      showSnackbar(`${state.selectedItems.length} SharedRDM resources deleted successfully`, 'success');
      setState(prev => ({ ...prev, selectedItems: [] }));
      loadSharedRDMs();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to delete SharedRDM resources';
      showSnackbar(errorMessage, 'error');
    }
  };

  const handleFormSubmit = async (spec: SharedRDMSpec) => {
    try {
      if (state.editingItem) {
        await updateSharedRDM(state.editingItem.metadata.name, spec);
        showSnackbar(`SharedRDM "${state.editingItem.metadata.name}" updated successfully`, 'success');
      } else {
        await createSharedRDM(spec);
        showSnackbar('SharedRDM created successfully', 'success');
      }
      setState(prev => ({ ...prev, formOpen: false, editingItem: null }));
      loadSharedRDMs();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to save SharedRDM';
      showSnackbar(errorMessage, 'error');
    }
  };

  const handleExport = () => {
    const dataStr = JSON.stringify(state.sharedRDMs, null, 2);
    const dataUri = 'data:application/json;charset=utf-8,'+ encodeURIComponent(dataStr);
    const exportFileDefaultName = 'shared-rdm-resources.json';
    
    const linkElement = document.createElement('a');
    linkElement.setAttribute('href', dataUri);
    linkElement.setAttribute('download', exportFileDefaultName);
    linkElement.click();
    
    showSnackbar('SharedRDM resources exported successfully', 'success');
  };

  const handleSelectAll = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.checked) {
      const newSelected = filteredRDMs.map(rdm => rdm.metadata.name);
      setState(prev => ({ ...prev, selectedItems: newSelected }));
    } else {
      setState(prev => ({ ...prev, selectedItems: [] }));
    }
  };

  const handleSelectItem = (name: string) => {
    const selectedIndex = state.selectedItems.indexOf(name);
    let newSelected: string[] = [];

    if (selectedIndex === -1) {
      newSelected = newSelected.concat(state.selectedItems, name);
    } else if (selectedIndex === 0) {
      newSelected = newSelected.concat(state.selectedItems.slice(1));
    } else if (selectedIndex === state.selectedItems.length - 1) {
      newSelected = newSelected.concat(state.selectedItems.slice(0, -1));
    } else if (selectedIndex > 0) {
      newSelected = newSelected.concat(
        state.selectedItems.slice(0, selectedIndex),
        state.selectedItems.slice(selectedIndex + 1),
      );
    }

    setState(prev => ({ ...prev, selectedItems: newSelected }));
  };

  const handleActionMenu = (event: React.MouseEvent<HTMLElement>, id: string) => {
    setState(prev => ({ ...prev, anchorEl: event.currentTarget, actionMenuId: id }));
  };

  const handleCloseActionMenu = () => {
    setState(prev => ({ ...prev, anchorEl: null, actionMenuId: null }));
  };

  const handleSearchChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setState(prev => ({ ...prev, searchTerm: event.target.value, page: 0 }));
  };

  const handleStatusFilterChange = (event: SelectChangeEvent) => {
    setState(prev => ({ ...prev, statusFilter: event.target.value, page: 0 }));
  };

  const handleChangePage = (event: unknown, newPage: number) => {
    setState(prev => ({ ...prev, page: newPage }));
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setState(prev => ({ ...prev, rowsPerPage: parseInt(event.target.value, 10), page: 0 }));
  };

  const filteredRDMs = state.sharedRDMs.filter(rdm => {
    const matchesSearch = rdm.metadata.name.toLowerCase().includes(state.searchTerm.toLowerCase()) ||
                         rdm.spec.displayName?.toLowerCase().includes(state.searchTerm.toLowerCase());
    const matchesStatus = !state.statusFilter || rdm.status?.phase === state.statusFilter;
    return matchesSearch && matchesStatus;
  });

  const paginatedRDMs = filteredRDMs.slice(
    state.page * state.rowsPerPage,
    state.page * state.rowsPerPage + state.rowsPerPage
  );

  const isSelected = (name: string) => state.selectedItems.indexOf(name) !== -1;
  const numSelected = state.selectedItems.length;
  const rowCount = filteredRDMs.length;

  return (
    <Box sx={{ p: 3 }}>
      {/* Header */}
      <Box sx={{ mb: 3, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h4" component="h1">
          Shared RDM Resources
        </Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={loadSharedRDMs}
            disabled={state.loading}
          >
            Refresh
          </Button>
          <Button
            variant="outlined"
            startIcon={<DownloadIcon />}
            onClick={handleExport}
            disabled={state.sharedRDMs.length === 0}
          >
            Export
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleCreate}
          >
            Create SharedRDM
          </Button>
        </Box>
      </Box>

      {/* Filters */}
      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                placeholder="Search SharedRDM resources..."
                value={state.searchTerm}
                onChange={handleSearchChange}
                InputProps={{
                  startAdornment: <SearchIcon sx={{ mr: 1, color: 'text.secondary' }} />,
                }}
              />
            </Grid>
            <Grid item xs={12} sm={6} md={3}>
              <FormControl fullWidth>
                <InputLabel>Status Filter</InputLabel>
                <Select
                  value={state.statusFilter}
                  label="Status Filter"
                  onChange={handleStatusFilterChange}
                >
                  <MenuItem value="">All Statuses</MenuItem>
                  <MenuItem value="Discovering">Discovering</MenuItem>
                  <MenuItem value="Creating">Creating</MenuItem>
                  <MenuItem value="Ready">Ready</MenuItem>
                  <MenuItem value="Failed">Failed</MenuItem>
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12} sm={12} md={5}>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1 }}>
                {numSelected > 0 && (
                  <Button
                    variant="outlined"
                    color="error"
                    startIcon={<DeleteIcon />}
                    onClick={handleBulkDelete}
                  >
                    Delete Selected ({numSelected})
                  </Button>
                )}
              </Box>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      {/* Error Alert */}
      {state.error && (
        <Alert severity="error" sx={{ mb: 3 }} onClose={() => setState(prev => ({ ...prev, error: null }))}>
          {state.error}
        </Alert>
      )}

      {/* Data Table */}
      <Paper>
        <TableContainer>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox">
                  <Checkbox
                    indeterminate={numSelected > 0 && numSelected < rowCount}
                    checked={rowCount > 0 && numSelected === rowCount}
                    onChange={handleSelectAll}
                  />
                </TableCell>
                <TableCell>Name</TableCell>
                <TableCell>Display Name</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Size</TableCell>
                <TableCell>VMs</TableCell>
                <TableCell>Volume ID</TableCell>
                <TableCell>Strategy</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {state.loading ? (
                <TableRow>
                  <TableCell colSpan={9} align="center" sx={{ py: 4 }}>
                    <CircularProgress />
                  </TableCell>
                </TableRow>
              ) : paginatedRDMs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} align="center" sx={{ py: 4 }}>
                    <Typography variant="body2" color="text.secondary">
                      {state.searchTerm || state.statusFilter ? 'No SharedRDM resources match your filters' : 'No SharedRDM resources found'}
                    </Typography>
                  </TableCell>
                </TableRow>
              ) : (
                paginatedRDMs.map((rdm) => {
                  const isItemSelected = isSelected(rdm.metadata.name);
                  return (
                    <TableRow
                      key={rdm.metadata.name}
                      hover
                      selected={isItemSelected}
                    >
                      <TableCell padding="checkbox">
                        <Checkbox
                          checked={isItemSelected}
                          onChange={() => handleSelectItem(rdm.metadata.name)}
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {rdm.metadata.name}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        {rdm.spec.displayName || '-'}
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={rdm.status?.phase || 'Unknown'}
                          color={getStatusColor(rdm.status?.phase || '')}
                          size="small"
                        />
                      </TableCell>
                      <TableCell>
                        {formatBytes(rdm.spec.diskSize)}
                      </TableCell>
                      <TableCell>
                        <Tooltip title={rdm.spec.vmwareRefs?.join(', ') || 'No VMs'}>
                          <Chip
                            label={`${rdm.spec.vmwareRefs?.length || 0} VMs`}
                            size="small"
                            variant="outlined"
                          />
                        </Tooltip>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                          {rdm.status?.volumeID || '-'}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        {rdm.spec.migrationStrategy || '-'}
                      </TableCell>
                      <TableCell align="right">
                        <Box sx={{ display: 'flex', gap: 0.5 }}>
                          <Tooltip title="View Details">
                            <IconButton size="small" onClick={() => handleView(rdm)}>
                              <ViewIcon />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Edit">
                            <IconButton size="small" onClick={() => handleEdit(rdm)}>
                              <EditIcon />
                            </IconButton>
                          </Tooltip>
                          <IconButton
                            size="small"
                            onClick={(e) => handleActionMenu(e, rdm.metadata.name)}
                          >
                            <MoreVertIcon />
                          </IconButton>
                        </Box>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          rowsPerPageOptions={[5, 10, 25, 50]}
          component="div"
          count={filteredRDMs.length}
          rowsPerPage={state.rowsPerPage}
          page={state.page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      </Paper>

      {/* Action Menu */}
      <Menu
        anchorEl={state.anchorEl}
        open={Boolean(state.anchorEl)}
        onClose={handleCloseActionMenu}
      >
        <MenuItem onClick={() => {
          const rdm = state.sharedRDMs.find(r => r.metadata.name === state.actionMenuId);
          if (rdm) handleView(rdm);
          handleCloseActionMenu();
        }}>
          <ViewIcon sx={{ mr: 1 }} />
          View Details
        </MenuItem>
        <MenuItem onClick={() => {
          const rdm = state.sharedRDMs.find(r => r.metadata.name === state.actionMenuId);
          if (rdm) handleEdit(rdm);
          handleCloseActionMenu();
        }}>
          <EditIcon sx={{ mr: 1 }} />
          Edit
        </MenuItem>
        <MenuItem onClick={() => {
          const rdm = state.sharedRDMs.find(r => r.metadata.name === state.actionMenuId);
          if (rdm) handleDelete(rdm);
          handleCloseActionMenu();
        }}>
          <DeleteIcon sx={{ mr: 1 }} />
          Delete
        </MenuItem>
      </Menu>

      {/* Create/Edit Form Dialog */}
      <Dialog
        open={state.formOpen}
        onClose={() => setState(prev => ({ ...prev, formOpen: false, editingItem: null }))}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          {state.editingItem ? 'Edit SharedRDM' : 'Create SharedRDM'}
        </DialogTitle>
        <DialogContent>
          <SharedRDMForm
            initialData={state.editingItem?.spec}
            onSubmit={handleFormSubmit}
            onCancel={() => setState(prev => ({ ...prev, formOpen: false, editingItem: null }))}
          />
        </DialogContent>
      </Dialog>

      {/* Details View Dialog */}
      <Dialog
        open={state.detailsOpen}
        onClose={() => setState(prev => ({ ...prev, detailsOpen: false, viewingItem: null }))}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          SharedRDM Details: {state.viewingItem?.metadata.name}
        </DialogTitle>
        <DialogContent>
          {state.viewingItem && (
            <Box sx={{ mt: 2 }}>
              <Grid container spacing={2}>
                <Grid item xs={12} sm={6}>
                  <Typography variant="subtitle2" gutterBottom>
                    Basic Information
                  </Typography>
                  <Box sx={{ pl: 2 }}>
                    <Typography variant="body2"><strong>Name:</strong> {state.viewingItem.metadata.name}</Typography>
                    <Typography variant="body2"><strong>Display Name:</strong> {state.viewingItem.spec.displayName || '-'}</Typography>
                    <Typography variant="body2"><strong>Source UUID:</strong> {state.viewingItem.spec.sourceUUID}</Typography>
                    <Typography variant="body2"><strong>Size:</strong> {formatBytes(state.viewingItem.spec.diskSize)}</Typography>
                    <Typography variant="body2"><strong>Migration Strategy:</strong> {state.viewingItem.spec.migrationStrategy || '-'}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12} sm={6}>
                  <Typography variant="subtitle2" gutterBottom>
                    Status Information
                  </Typography>
                  <Box sx={{ pl: 2 }}>
                    <Typography variant="body2">
                      <strong>Phase:</strong>{' '}
                      <Chip
                        label={state.viewingItem.status?.phase || 'Unknown'}
                        color={getStatusColor(state.viewingItem.status?.phase || '')}
                        size="small"
                      />
                    </Typography>
                    <Typography variant="body2"><strong>Volume ID:</strong> {state.viewingItem.status?.volumeID || '-'}</Typography>
                    <Typography variant="body2"><strong>Backend Pool:</strong> {state.viewingItem.spec.cinderBackendPool || '-'}</Typography>
                    <Typography variant="body2"><strong>Volume Type:</strong> {state.viewingItem.spec.volumeType || '-'}</Typography>
                  </Box>
                </Grid>
                <Grid item xs={12}>
                  <Typography variant="subtitle2" gutterBottom>
                    Associated VMs
                  </Typography>
                  <Box sx={{ pl: 2 }}>
                    {state.viewingItem.spec.vmwareRefs?.length ? (
                      state.viewingItem.spec.vmwareRefs.map((vm, index) => (
                        <Chip key={index} label={vm} sx={{ mr: 1, mb: 1 }} />
                      ))
                    ) : (
                      <Typography variant="body2" color="text.secondary">No VMs associated</Typography>
                    )}
                  </Box>
                </Grid>
                {state.viewingItem.status?.conditions && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" gutterBottom>
                      Conditions
                    </Typography>
                    <Box sx={{ pl: 2 }}>
                      {state.viewingItem.status.conditions.map((condition, index) => (
                        <Box key={index} sx={{ mb: 1 }}>
                          <Typography variant="body2">
                            <strong>{condition.type}:</strong> {condition.status} - {condition.reason}
                          </Typography>
                          {condition.message && (
                            <Typography variant="body2" color="text.secondary" sx={{ pl: 2 }}>
                              {condition.message}
                            </Typography>
                          )}
                        </Box>
                      ))}
                    </Box>
                  </Grid>
                )}
              </Grid>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setState(prev => ({ ...prev, detailsOpen: false, viewingItem: null }))}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* Snackbar */}
      <Snackbar
        open={state.snackbarOpen}
        autoHideDuration={6000}
        onClose={() => setState(prev => ({ ...prev, snackbarOpen: false }))}
      >
        <Alert
          onClose={() => setState(prev => ({ ...prev, snackbarOpen: false }))}
          severity={state.snackbarSeverity}
          sx={{ width: '100%' }}
        >
          {state.snackbarMessage}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default SharedRDMPage;