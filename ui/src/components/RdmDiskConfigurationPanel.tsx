import React, { useState, useEffect, useRef } from 'react'
import {
    Box,
    Typography,
    Card,
    CardContent,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    Chip,
    Divider,
    Alert,
    Grid,
    Tooltip,
    TextField,
} from '@mui/material'
// Icons removed since source fields are now readonly
import { RdmDisk } from 'src/api/rdm-disks/model'
import { OpenstackCreds } from 'src/api/openstack-creds/model'

interface RdmDiskConfiguration {
    uuid: string
    diskName: string
    cinderBackendPool: string
    volumeType: string
    source: { [key: string]: string }
}

interface RdmDiskConfigurationPanelProps {
    rdmDisks: RdmDisk[]
    openstackCreds?: OpenstackCreds
    selectedVMs: string[]
    onConfigurationChange: (configurations: RdmDiskConfiguration[]) => void
}

export const RdmDiskConfigurationPanel: React.FC<RdmDiskConfigurationPanelProps> = ({
    rdmDisks,
    openstackCreds,
    selectedVMs,
    onConfigurationChange,
}) => {
    const [configurations, setConfigurations] = useState<RdmDiskConfiguration[]>([])
    const initializedRef = useRef(false)

    // Initialize config when rdmDisks change
    useEffect(() => {
        if (rdmDisks.length > 0 && !initializedRef.current) {
            const initialConfigs = rdmDisks.map(disk => ({
                uuid: disk.spec.uuid,
                diskName: disk.spec.diskName,
                cinderBackendPool: disk.spec.openstackVolumeRef?.cinderBackendPool || '',
                volumeType: disk.spec.openstackVolumeRef?.volumeType || '',
                source: disk.spec.openstackVolumeRef?.source || {},
            }))
            setConfigurations(initialConfigs)
            initializedRef.current = true
        }
    }, [rdmDisks])

    // Notify parent when configurations change
    useEffect(() => {
        onConfigurationChange(configurations)
    }, [configurations, onConfigurationChange])

    const updateConfiguration = (index: number, field: keyof RdmDiskConfiguration, value: string | Record<string, string>) => {
        setConfigurations(prev => {
            const updated = [...prev]
            updated[index] = { ...updated[index], [field]: value }
            return updated
        })
    }

    // Source fields are now readonly, so add/update/remove functions are no longer needed


    const availableBackendPools = openstackCreds?.status?.openstack?.volumeBackends || []
    const availableVolumeTypes = openstackCreds?.status?.openstack?.volumeTypes || []

    if (rdmDisks.length === 0) {
        return (
            <Box sx={{ p: 2, textAlign: 'center' }}>
                <Typography color="text.secondary">
                    No RDM disks available for configuration.
                </Typography>
            </Box>
        )
    }

    return (
        <Box sx={{ mt: 3 }}>
            <Typography variant="h6" sx={{ mb: 2, display: 'flex', alignItems: 'center' }}>
                🔗 RDM Disk Configuration
            </Typography>

            <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="body2">
                    Configure OpenStack Cinder settings for each RDM disk. These settings will be applied when migrating the selected VMs.
                </Typography>
            </Alert>

            <Box sx={{ mb: 2 }}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                    Selected VMs:
                </Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                    {selectedVMs.map(vmName => (
                        <Chip key={vmName} label={vmName} size="small" color="primary" variant="outlined" />
                    ))}
                </Box>
            </Box>

            {rdmDisks.map((disk, index) => {
                const config = configurations[index]
                if (!config) return null

                const ownerVMs = disk.spec.ownerVMs
                const diskSizeGB = Math.round(disk.spec.diskSize / (1024 * 1024 * 1024))

                return (
                    <Card key={disk.metadata.name} sx={{ mb: 2 }}>
                        <CardContent>
                            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 'medium' }}>
                                    {disk.spec.displayName}
                                </Typography>
                                <Chip
                                    label={`${diskSizeGB} GB`}
                                    size="small"
                                    color="secondary"
                                    variant="outlined"
                                />
                            </Box>

                            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                                Disk Name: {disk.spec.diskName} • UUID: {disk.spec.uuid}
                            </Typography>

                            <Box sx={{ mb: 2 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                    Owner VMs:
                                </Typography>
                                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                                    {ownerVMs.map(vmName => {
                                        const isSelected = selectedVMs.includes(vmName)
                                        const chipElement = (
                                            <Chip
                                                key={vmName}
                                                label={vmName}
                                                size="small"
                                                color={isSelected ? "primary" : "default"}
                                                variant={isSelected ? "filled" : "outlined"}
                                            />
                                        );

                                        // Show tooltip for unselected VMs
                                        if (!isSelected) {
                                            return (
                                                <Tooltip key={vmName} title={`${vmName} also shares this RDM disk (not selected for migration)`}>
                                                    {chipElement}
                                                </Tooltip>
                                            );
                                        }

                                        return chipElement;
                                    })}
                                </Box>
                            </Box>

                            <Divider sx={{ my: 2 }} />

                            <Grid container spacing={2} sx={{ mb: 2 }}>
                                <Grid item xs={12} md={6}>
                                    <FormControl fullWidth size="small">
                                        <InputLabel>Cinder Backend Pool *</InputLabel>
                                        <Select
                                            value={config.cinderBackendPool}
                                            label="Cinder Backend Pool *"
                                            onChange={(e) => updateConfiguration(index, 'cinderBackendPool', e.target.value)}
                                        >
                                            {availableBackendPools.map(pool => (
                                                <MenuItem key={pool} value={pool}>
                                                    {pool}
                                                </MenuItem>
                                            ))}
                                        </Select>
                                    </FormControl>
                                </Grid>

                                <Grid item xs={12} md={6}>
                                    <FormControl fullWidth size="small">
                                        <InputLabel>Volume Type *</InputLabel>
                                        <Select
                                            value={config.volumeType}
                                            label="Volume Type *"
                                            onChange={(e) => updateConfiguration(index, 'volumeType', e.target.value)}
                                        >
                                            {availableVolumeTypes.map(type => (
                                                <MenuItem key={type} value={type}>
                                                    {type}
                                                </MenuItem>
                                            ))}
                                        </Select>
                                    </FormControl>
                                </Grid>
                            </Grid>

                            <Box>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                                    <Typography variant="body2" sx={{ fontWeight: 'medium' }}>
                                        Volume Ref Configuration
                                    </Typography>
                                </Box>

                                <Alert severity="info" sx={{ mb: 2 }}>
                                    <Typography variant="body2">
                                        <strong>Hint:</strong> Edit this field in the VMware Notes section.
                                        <br />
                                        Multipath SAN support is available only in PCD ≥ 2025.10.
                                    </Typography>
                                </Alert>

                                {Object.entries(config.source).map(([key, value], sourceIndex) => (
                                    <Grid container spacing={1} alignItems="center" key={`${index}-${sourceIndex}`} sx={{ mb: 1 }}>
                                        <Grid item xs={5}>
                                            <TextField
                                                size="small"
                                                placeholder="Key"
                                                value={key}
                                                fullWidth
                                                InputProps={{
                                                    readOnly: true,
                                                }}
                                                sx={{
                                                    '& .MuiInputBase-input': {
                                                        backgroundColor: 'action.disabledBackground',
                                                    }
                                                }}
                                            />
                                        </Grid>
                                        <Grid item xs={5}>
                                            <TextField
                                                size="small"
                                                placeholder="Value"
                                                value={value}
                                                fullWidth
                                                InputProps={{
                                                    readOnly: true,
                                                }}
                                                sx={{
                                                    '& .MuiInputBase-input': {
                                                        backgroundColor: 'action.disabledBackground',
                                                    }
                                                }}
                                            />
                                        </Grid>
                                        <Grid item xs={2}>
                                            {/* Removed delete button since fields are now readonly */}
                                        </Grid>
                                    </Grid>
                                ))}

                                {Object.keys(config.source).length === 0 && (
                                    <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                                        No volume ref configuration fields available
                                    </Typography>
                                )}
                            </Box>
                        </CardContent>
                    </Card>
                )
            })}

        </Box>
    )
}
