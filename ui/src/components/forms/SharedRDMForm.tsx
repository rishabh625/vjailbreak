import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Card,
  CardContent,
  CardHeader,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  OutlinedInput,
  FormHelperText,
  Button,
  Grid,
  Typography,
  Alert,
  CircularProgress,
  Tooltip,
  IconButton,
  InputAdornment,
  Autocomplete,
  FormControlLabel,
  Switch,
  LinearProgress,
  Divider,
  Badge,
} from '@mui/material';
import {
  Save as SaveIcon,
  Cancel as CancelIcon,
  Info as InfoIcon,
  Refresh as RefreshIcon,
  Storage as StorageIcon,
  Computer as ComputerIcon,
} from '@mui/icons-material';
import { useFormik } from 'formik';
import * as Yup from 'yup';

// Types
interface SharedRDMSpec {
  sourceUUID: string;
  displayName: string;
  diskSize: number;
  vmwareRefs: string[];
  migrationStrategy: 'copy' | 'reuse' | 'manual';
  cinderBackendPool: string;
  volumeType: string;
}

interface SharedRDMStatus {
  phase: 'Discovering' | 'Creating' | 'Ready' | 'Failed';
  conditions: Array<{
    type: string;
    status: string;
    message: string;
    lastTransitionTime: string;
  }>;
  volumeID?: string;
  attachedVMs: string[];
}

interface SharedRDM {
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp?: string;
  };
  spec: SharedRDMSpec;
  status?: SharedRDMStatus;
}

interface BackendPool {
  name: string;
  capabilities: string[];
  totalCapacity: number;
  freeCapacity: number;
}

interface VolumeType {
  name: string;
  description: string;
  supportsMultiAttach: boolean;
  backendName: string;
}

interface VMwareVM {
  name: string;
  uuid: string;
  hasRDM: boolean;
  rdmDisks: Array<{
    uuid: string;
    size: number;
    isShared: boolean;
  }>;
}

interface SharedRDMFormProps {
  sharedRDM?: SharedRDM;
  mode: 'create' | 'edit' | 'view';
  onSave: (spec: SharedRDMSpec) => Promise<void>;
  onCancel: () => void;
  loading?: boolean;
}

// Validation schema
const validationSchema = Yup.object({
  sourceUUID: Yup.string()
    .required('Source UUID is required')
    .matches(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
      'Invalid UUID format'
    ),
  displayName: Yup.string()
    .required('Display name is required')
    .min(3, 'Display name must be at least 3 characters')
    .max(63, 'Display name must be less than 64 characters')
    .matches(
      /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/,
      'Display name must be a valid Kubernetes name'
    ),
  diskSize: Yup.number()
    .required('Disk size is required')
    .min(1024 * 1024 * 1024, 'Minimum disk size is 1GB')
    .max(1024 * 1024 * 1024 * 1024 * 10, 'Maximum disk size is 10TB'),
  vmwareRefs: Yup.array()
    .of(Yup.string())
    .min(1, 'At least one VM reference is required')
    .max(10, 'Maximum 10 VM references allowed'),
  migrationStrategy: Yup.string()
    .required('Migration strategy is required')
    .oneOf(['copy', 'reuse', 'manual'], 'Invalid migration strategy'),
  cinderBackendPool: Yup.string().required('Backend pool is required'),
  volumeType: Yup.string().required('Volume type is required'),
});

// Utility functions
const formatBytes = (bytes: number): string => {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unitIndex = 0;
  
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  
  return `${size.toFixed(2)} ${units[unitIndex]}`;
};

const parseSize = (value: string, unit: string): number => {
  const num = parseFloat(value);
  const multipliers = {
    B: 1,
    KB: 1024,
    MB: 1024 * 1024,
    GB: 1024 * 1024 * 1024,
    TB: 1024 * 1024 * 1024 * 1024,
  };
  return num * (multipliers[unit as keyof typeof multipliers] || 1);
};

const getPhaseColor = (phase: string): 'default' | 'primary' | 'secondary' | 'error' | 'info' | 'success' | 'warning' => {
  switch (phase) {
    case 'Ready': return 'success';
    case 'Creating': return 'info';
    case 'Discovering': return 'warning';
    case 'Failed': return 'error';
    default: return 'default';
  }
};

export const SharedRDMForm: React.FC<SharedRDMFormProps> = ({
  sharedRDM,
  mode,
  onSave,
  onCancel,
  loading = false,
}) => {
  // State
  const [backendPools, setBackendPools] = useState<BackendPool[]>([]);
  const [volumeTypes, setVolumeTypes] = useState<VolumeType[]>([]);
  const [vmwareVMs, setVmwareVMs] = useState<VMwareVM[]>([]);
  const [loadingPools, setLoadingPools] = useState(false);
  const [loadingTypes, setLoadingTypes] = useState(false);
  const [loadingVMs, setLoadingVMs] = useState(false);
  const [diskSizeValue, setDiskSizeValue] = useState('');
  const [diskSizeUnit, setDiskSizeUnit] = useState('GB');
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Form setup
  const formik = useFormik({
    initialValues: {
      sourceUUID: sharedRDM?.spec.sourceUUID || '',
      displayName: sharedRDM?.spec.displayName || '',
      diskSize: sharedRDM?.spec.diskSize || 0,
      vmwareRefs: sharedRDM?.spec.vmwareRefs || [],
      migrationStrategy: sharedRDM?.spec.migrationStrategy || 'copy' as const,
      cinderBackendPool: sharedRDM?.spec.cinderBackendPool || '',
      volumeType: sharedRDM?.spec.volumeType || '',
    },
    validationSchema,
    onSubmit: async (values) => {
      await onSave(values);
    },
  });

  // Load data
  const loadBackendPools = useCallback(async () => {
    setLoadingPools(true);
    try {
      // Mock data - replace with actual API call
      const pools: BackendPool[] = [
        {
          name: 'ssd-pool',
          capabilities: ['multi-attach', 'encryption'],
          totalCapacity: 1024 * 1024 * 1024 * 1024,
          freeCapacity: 512 * 1024 * 1024 * 1024,
        },
        {
          name: 'hdd-pool',
          capabilities: ['multi-attach'],
          totalCapacity: 10 * 1024 * 1024 * 1024 * 1024,
          freeCapacity: 8 * 1024 * 1024 * 1024 * 1024,
        },
      ];
      setBackendPools(pools);
    } catch (error) {
      console.error('Failed to load backend pools:', error);
    } finally {
      setLoadingPools(false);
    }
  }, []);

  const loadVolumeTypes = useCallback(async () => {
    setLoadingTypes(true);
    try {
      // Mock data - replace with actual API call
      const types: VolumeType[] = [
        {
          name: 'high-performance',
          description: 'High performance SSD storage',
          supportsMultiAttach: true,
          backendName: 'ssd-pool',
        },
        {
          name: 'standard',
          description: 'Standard HDD storage',
          supportsMultiAttach: true,
          backendName: 'hdd-pool',
        },
        {
          name: 'encrypted',
          description: 'Encrypted SSD storage',
          supportsMultiAttach: false,
          backendName: 'ssd-pool',
        },
      ];
      setVolumeTypes(types);
    } catch (error) {
      console.error('Failed to load volume types:', error);
    } finally {
      setLoadingTypes(false);
    }
  }, []);

  const loadVMwareVMs = useCallback(async () => {
    setLoadingVMs(true);
    try {
      // Mock data - replace with actual API call
      const vms: VMwareVM[] = [
        {
          name: 'database-vm-1',
          uuid: 'vm-001',
          hasRDM: true,
          rdmDisks: [
            { uuid: formik.values.sourceUUID, size: 100 * 1024 * 1024 * 1024, isShared: true },
          ],
        },
        {
          name: 'database-vm-2',
          uuid: 'vm-002',
          hasRDM: true,
          rdmDisks: [
            { uuid: formik.values.sourceUUID, size: 100 * 1024 * 1024 * 1024, isShared: true },
          ],
        },
        {
          name: 'web-vm-1',
          uuid: 'vm-003',
          hasRDM: false,
          rdmDisks: [],
        },
      ];
      setVmwareVMs(vms);
    } catch (error) {
      console.error('Failed to load VMware VMs:', error);
    } finally {
      setLoadingVMs(false);
    }
  }, [formik.values.sourceUUID]);

  // Effects
  useEffect(() => {
    loadBackendPools();
    loadVolumeTypes();
  }, [loadBackendPools, loadVolumeTypes]);

  useEffect(() => {
    if (formik.values.sourceUUID) {
      loadVMwareVMs();
    }
  }, [formik.values.sourceUUID, loadVMwareVMs]);

  useEffect(() => {
    if (sharedRDM?.spec.diskSize) {
      const size = sharedRDM.spec.diskSize;
      if (size >= 1024 * 1024 * 1024 * 1024) {
        setDiskSizeValue((size / (1024 * 1024 * 1024 * 1024)).toString());
        setDiskSizeUnit('TB');
      } else if (size >= 1024 * 1024 * 1024) {
        setDiskSizeValue((size / (1024 * 1024 * 1024)).toString());
        setDiskSizeUnit('GB');
      } else {
        setDiskSizeValue((size / (1024 * 1024)).toString());
        setDiskSizeUnit('MB');
      }
    }
  }, [sharedRDM]);

  // Handlers
  const handleDiskSizeChange = (value: string, unit: string) => {
    setDiskSizeValue(value);
    setDiskSizeUnit(unit);
    const bytes = parseSize(value, unit);
    formik.setFieldValue('diskSize', bytes);
  };

  const handleRefresh = () => {
    loadBackendPools();
    loadVolumeTypes();
    loadVMwareVMs();
  };

  const isReadOnly = mode === 'view';
  const filteredVolumeTypes = volumeTypes.filter(type => 
    !formik.values.cinderBackendPool || type.backendName === formik.values.cinderBackendPool
  );

  return (
    <Card>
      <CardHeader
        title={
          <Box display="flex" alignItems="center" gap={1}>
            <StorageIcon />
            <Typography variant="h6">
              {mode === 'create' ? 'Create' : mode === 'edit' ? 'Edit' : 'View'} Shared RDM
            </Typography>
            {sharedRDM?.status && (
              <Chip
                label={sharedRDM.status.phase}
                color={getPhaseColor(sharedRDM.status.phase)}
                size="small"
              />
            )}
          </Box>
        }
        action={
          <Box display="flex" gap={1}>
            <Tooltip title="Refresh data">
              <IconButton onClick={handleRefresh} disabled={loading}>
                <RefreshIcon />
              </IconButton>
            </Tooltip>
            <FormControlLabel
              control={
                <Switch
                  checked={showAdvanced}
                  onChange={(e) => setShowAdvanced(e.target.checked)}
                  size="small"
                />
              }
              label="Advanced"
            />
          </Box>
        }
      />
      
      <CardContent>
        {loading && <LinearProgress sx={{ mb: 2 }} />}
        
        {sharedRDM?.status?.conditions?.some(c => c.type === 'Error') && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {sharedRDM.status.conditions.find(c => c.type === 'Error')?.message}
          </Alert>
        )}

        <form onSubmit={formik.handleSubmit}>
          <Grid container spacing={3}>
            {/* Basic Information */}
            <Grid item xs={12}>
              <Typography variant="h6" gutterBottom>
                Basic Information
              </Typography>
            </Grid>

            <Grid item xs={12} md={6}>
              <TextField
                fullWidth
                label="Source UUID"
                name="sourceUUID"
                value={formik.values.sourceUUID}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                error={formik.touched.sourceUUID && Boolean(formik.errors.sourceUUID)}
                helperText={formik.touched.sourceUUID && formik.errors.sourceUUID}
                disabled={isReadOnly}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title="VMware LUN UUID that identifies the raw device mapping">
                        <InfoIcon color="action" />
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
            </Grid>

            <Grid item xs={12} md={6}>
              <TextField
                fullWidth
                label="Display Name"
                name="displayName"
                value={formik.values.displayName}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                error={formik.touched.displayName && Boolean(formik.errors.displayName)}
                helperText={formik.touched.displayName && formik.errors.displayName}
                disabled={isReadOnly}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title="Human-readable name for the shared RDM resource">
                        <InfoIcon color="action" />
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
            </Grid>

            <Grid item xs={12} md={6}>
              <Box display="flex" gap={1}>
                <TextField
                  label="Disk Size"
                  value={diskSizeValue}
                  onChange={(e) => handleDiskSizeChange(e.target.value, diskSizeUnit)}
                  error={formik.touched.diskSize && Boolean(formik.errors.diskSize)}
                  disabled={isReadOnly}
                  type="number"
                  sx={{ flex: 1 }}
                />
                <FormControl sx={{ minWidth: 80 }}>
                  <InputLabel>Unit</InputLabel>
                  <Select
                    value={diskSizeUnit}
                    onChange={(e) => handleDiskSizeChange(diskSizeValue, e.target.value)}
                    disabled={isReadOnly}
                    label="Unit"
                  >
                    <MenuItem value="MB">MB</MenuItem>
                    <MenuItem value="GB">GB</MenuItem>
                    <MenuItem value="TB">TB</MenuItem>
                  </Select>
                </FormControl>
              </Box>
              {formik.touched.diskSize && formik.errors.diskSize && (
                <FormHelperText error>{formik.errors.diskSize}</FormHelperText>
              )}
            </Grid>

            <Grid item xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel>Migration Strategy</InputLabel>
                <Select
                  name="migrationStrategy"
                  value={formik.values.migrationStrategy}
                  onChange={formik.handleChange}
                  onBlur={formik.handleBlur}
                  error={formik.touched.migrationStrategy && Boolean(formik.errors.migrationStrategy)}
                  disabled={isReadOnly}
                  label="Migration Strategy"
                >
                  <MenuItem value="copy">Copy - Copy data to new volume</MenuItem>
                  <MenuItem value="reuse">Reuse - Use existing volume</MenuItem>
                  <MenuItem value="manual">Manual - Manual migration</MenuItem>
                </Select>
                {formik.touched.migrationStrategy && formik.errors.migrationStrategy && (
                  <FormHelperText error>{formik.errors.migrationStrategy}</FormHelperText>
                )}
              </FormControl>
            </Grid>

            {/* VMware References */}
            <Grid item xs={12}>
              <Divider sx={{ my: 2 }} />
              <Typography variant="h6" gutterBottom>
                VMware VM References
              </Typography>
            </Grid>

            <Grid item xs={12}>
              <Autocomplete
                multiple
                options={vmwareVMs.filter(vm => vm.hasRDM)}
                getOptionLabel={(option) => option.name}
                value={vmwareVMs.filter(vm => formik.values.vmwareRefs.includes(vm.name))}
                onChange={(_, newValue) => {
                  formik.setFieldValue('vmwareRefs', newValue.map(vm => vm.name));
                }}
                disabled={isReadOnly || loadingVMs}
                loading={loadingVMs}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="VMware VMs"
                    error={formik.touched.vmwareRefs && Boolean(formik.errors.vmwareRefs)}
                    helperText={formik.touched.vmwareRefs && formik.errors.vmwareRefs}
                    InputProps={{
                      ...params.InputProps,
                      endAdornment: (
                        <>
                          {loadingVMs && <CircularProgress color="inherit" size={20} />}
                          {params.InputProps.endAdornment}
                        </>
                      ),
                    }}
                  />
                )}
                renderTags={(value, getTagProps) =>
                  value.map((option, index) => (
                    <Chip
                      variant="outlined"
                      label={option.name}
                      icon={<ComputerIcon />}
                      {...getTagProps({ index })}
                      key={option.uuid}
                    />
                  ))
                }
                renderOption={(props, option) => (
                  <Box component="li" {...props}>
                    <Box display="flex" alignItems="center" gap={1} width="100%">
                      <ComputerIcon />
                      <Box flex={1}>
                        <Typography variant="body2">{option.name}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {option.rdmDisks.length} RDM disk(s)
                        </Typography>
                      </Box>
                    </Box>
                  </Box>
                )}
              />
            </Grid>

            {/* Storage Configuration */}
            <Grid item xs={12}>
              <Divider sx={{ my: 2 }} />
              <Typography variant="h6" gutterBottom>
                Storage Configuration
              </Typography>
            </Grid>

            <Grid item xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel>Backend Pool</InputLabel>
                <Select
                  name="cinderBackendPool"
                  value={formik.values.cinderBackendPool}
                  onChange={formik.handleChange}
                  onBlur={formik.handleBlur}
                  error={formik.touched.cinderBackendPool && Boolean(formik.errors.cinderBackendPool)}
                  disabled={isReadOnly || loadingPools}
                  label="Backend Pool"
                >
                  {backendPools.map((pool) => (
                    <MenuItem key={pool.name} value={pool.name}>
                      <Box>
                        <Typography variant="body2">{pool.name}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          Free: {formatBytes(pool.freeCapacity)} / {formatBytes(pool.totalCapacity)}
                        </Typography>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
                {formik.touched.cinderBackendPool && formik.errors.cinderBackendPool && (
                  <FormHelperText error>{formik.errors.cinderBackendPool}</FormHelperText>
                )}
              </FormControl>
            </Grid>

            <Grid item xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel>Volume Type</InputLabel>
                <Select
                  name="volumeType"
                  value={formik.values.volumeType}
                  onChange={formik.handleChange}
                  onBlur={formik.handleBlur}
                  error={formik.touched.volumeType && Boolean(formik.errors.volumeType)}
                  disabled={isReadOnly || loadingTypes}
                  label="Volume Type"
                >
                  {filteredVolumeTypes.map((type) => (
                    <MenuItem key={type.name} value={type.name}>
                      <Box display="flex" alignItems="center" gap={1} width="100%">
                        <Box flex={1}>
                          <Typography variant="body2">{type.name}</Typography>
                          <Typography variant="caption" color="text.secondary">
                            {type.description}
                          </Typography>
                        </Box>
                        {type.supportsMultiAttach && (
                          <Chip label="Multi-attach" size="small" color="primary" />
                        )}
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
                {formik.touched.volumeType && formik.errors.volumeType && (
                  <FormHelperText error>{formik.errors.volumeType}</FormHelperText>
                )}
              </FormControl>
            </Grid>

            {/* Advanced Configuration */}
            {showAdvanced && (
              <>
                <Grid item xs={12}>
                  <Divider sx={{ my: 2 }} />
                  <Typography variant="h6" gutterBottom>
                    Advanced Configuration
                  </Typography>
                </Grid>

                {sharedRDM?.status && (
                  <Grid item xs={12}>
                    <Box>
                      <Typography variant="subtitle2" gutterBottom>
                        Status Information
                      </Typography>
                      <Box display="flex" gap={2} flexWrap="wrap">
                        <Chip
                          label={`Phase: ${sharedRDM.status.phase}`}
                          color={getPhaseColor(sharedRDM.status.phase)}
                        />
                        {sharedRDM.status.volumeID && (
                          <Chip
                            label={`Volume ID: ${sharedRDM.status.volumeID}`}
                            variant="outlined"
                          />
                        )}
                        <Chip
                          label={`Attached VMs: ${sharedRDM.status.attachedVMs.length}`}
                          variant="outlined"
                        />
                      </Box>
                    </Box>
                  </Grid>
                )}
              </>
            )}

            {/* Actions */}
            <Grid item xs={12}>
              <Divider sx={{ my: 2 }} />
              <Box display="flex" gap={2} justifyContent="flex-end">
                <Button
                  variant="outlined"
                  onClick={onCancel}
                  startIcon={<CancelIcon />}
                >
                  Cancel
                </Button>
                {!isReadOnly && (
                  <Button
                    type="submit"
                    variant="contained"
                    disabled={loading || !formik.isValid}
                    startIcon={loading ? <CircularProgress size={20} /> : <SaveIcon />}
                  >
                    {mode === 'create' ? 'Create' : 'Save'}
                  </Button>
                )}
              </Box>
            </Grid>
          </Grid>
        </form>
      </CardContent>
    </Card>
  );
};

export default SharedRDMForm;