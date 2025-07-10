import { useState, useEffect } from 'react';
import React from 'react';
import {
    Box,
    TextField,
    Button,
    Typography,
    Grid,
    styled,
    Paper,
    Select,
    MenuItem,
    FormControl,
    SelectChangeEvent,
    Switch,
    CircularProgress,
    Snackbar,
    Alert,
    Tooltip,
    useTheme
} from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
import CodeMirror from '@uiw/react-codemirror';
import { useErrorHandler } from "../../hooks/useErrorHandler";
import { yaml } from '@codemirror/lang-yaml';
import { EditorView } from '@codemirror/view';
import { createBmconfigSecret, deleteSecret, getSecret } from '../../api/secrets/secrets';
import {
    createBMConfigWithSecret,
    deleteBMConfig,
    getBMConfigList,
    getBMConfig,
    fetchBootSources,
    BootSourceSelection
} from '../../api/bmconfig/bmconfig';
import RefreshIcon from '@mui/icons-material/Refresh';
import {
    LIGHT_BG_DEFAULT,
    DARK_BG_ELEVATED,
    LIGHT_TEXT_PRIMARY,
    DARK_TEXT_PRIMARY,
    WHITE,
    BLACK
} from '../../theme/colors';

const StyledPaper = styled(Paper)({
    width: '100%',
    padding: '24px',
    boxSizing: 'border-box',
});

const Footer = styled(Box)(({ theme }) => ({
    display: 'flex',
    justifyContent: 'flex-end',
    gap: theme.spacing(2),
    marginTop: theme.spacing(4),
    paddingTop: theme.spacing(2),
    borderTop: `1px solid ${theme.palette.divider}`,
}));

const Section = styled(Box)(({ theme }) => ({
    marginBottom: theme.spacing(4),
}));

const FormField = styled(Box)(({ theme }) => ({
    marginBottom: theme.spacing(2),
}));

const CodeMirrorContainer = styled(Box)(({ theme }) => ({
    border: `1px solid ${theme.palette.grey[300]}`,
    borderRadius: theme.shape.borderRadius,
    overflow: 'hidden',
    marginBottom: theme.spacing(2),
}));

export default function MaasConfigForm() {
    const { reportError } = useErrorHandler({ component: "BMConfigForm" });
    const defaultCloudInit = `#cloud-config

# Run the cloud-init script on boot
runcmd:
  - echo "Hello World" > /root/hello-cloud-init`;

    interface FormDataType {
        maasUrl: string;
        insecure: boolean;
        apiKey: string;
        os: string;
        configName: string;
        namespace: string;
        cloudInit: string;
    }

    const [formData, setFormData] = useState<FormDataType>({
        maasUrl: '',
        insecure: false,
        apiKey: '',
        os: '',
        configName: 'bmconfig',
        namespace: 'migration-system',
        cloudInit: defaultCloudInit
    });

    const [loading, setLoading] = useState(false);
    const [initialLoading, setInitialLoading] = useState(true);
    const [submitting, setSubmitting] = useState(false);
    const [bootSources, setBootSources] = useState<BootSourceSelection[]>([]);
    const [notification, setNotification] = useState({
        open: false,
        message: '',
        severity: 'info' as 'error' | 'info' | 'success' | 'warning'
    });
    const [urlError, setUrlError] = useState('');

    const maasUrlRegex = /^https?:\/\/.+\/MAAS\/?$/;

    const theme = useTheme();
    const isDarkMode = theme.palette.mode === 'dark';

    const extensions = React.useMemo(() => [
        yaml(),
        EditorView.lineWrapping,
        EditorView.theme({
            '&': {
                fontSize: '14px',
            },
            '.cm-gutters': {
                backgroundColor: isDarkMode ? DARK_BG_ELEVATED : LIGHT_BG_DEFAULT,
                color: isDarkMode ? DARK_TEXT_PRIMARY : LIGHT_TEXT_PRIMARY,
                border: 'none',
            },
            '.cm-content': {
                caretColor: isDarkMode ? WHITE : BLACK,
            },
            '&.cm-focused .cm-cursor': {
                borderLeftColor: isDarkMode ? WHITE : BLACK,
            },
            '.cm-line': {
                padding: '0 4px',
            },
        })
    ], [isDarkMode]);

    useEffect(() => {
        fetchExistingMaasConfig();
    }, []);

    const fetchExistingMaasConfig = async () => {
        setInitialLoading(true);
        try {
            const configs = await getBMConfigList(formData.namespace);

            if (configs && configs.length > 0) {
                const config = await getBMConfig(configs[0].metadata.name, formData.namespace);

                if (config && config.spec) {
                    let maasUrl = '';
                    let insecure = config.spec.insecure || false;

                    if (config.spec.apiUrl) {
                        maasUrl = config.spec.apiUrl;
                        insecure = config.spec.insecure || false;
                    }

                    let cloudInitData = '';
                    if (config.spec.userDataSecretRef && config.spec.userDataSecretRef.name) {
                        try {
                            const secretData = await getSecret(
                                config.spec.userDataSecretRef.name,
                                config.spec.userDataSecretRef.namespace || formData.namespace
                            );

                            if (secretData && secretData.data && secretData.data["user-data"]) {
                                cloudInitData = secretData.data["user-data"];
                            }
                        } catch (error) {
                            console.warn('Error fetching user-data secret:', error);
                        }
                    }

                    setFormData({
                        maasUrl,
                        insecure,
                        apiKey: config.spec.apiKey || '',
                        os: config.spec.os || '',
                        configName: config.metadata.name,
                        namespace: config.metadata.namespace,
                        cloudInit: cloudInitData
                    });
                }
            }
        } catch (error) {
            console.error('Error fetching existing MaasConfig:', error);
        } finally {
            setInitialLoading(false);

            if (formData.maasUrl && formData.apiKey) {
                handleFetchBootSources();
            }
        }
    };

    useEffect(() => {
        if (formData.maasUrl && formData.apiKey && !urlError) {
            handleFetchBootSources();
        }
    }, [formData.maasUrl, formData.apiKey, urlError]);

    const handleFetchBootSources = async () => {
        if (!formData.maasUrl || !formData.apiKey || urlError) {
            return;
        }

        setLoading(true);
        try {
            const response = await fetchBootSources(
                formData.maasUrl,
                formData.apiKey,
                formData.insecure
            );

            setBootSources(response.bootSourceSelections);

            if (response.bootSourceSelections.length > 0) {
                const ubuntuJammy = response.bootSourceSelections.find(
                    source => source.OS === 'ubuntu' && source.Release === 'jammy'
                );

                if (ubuntuJammy) {
                    setFormData(prev => ({
                        ...prev,
                        os: ubuntuJammy.Release
                    }));
                } else if (response.bootSourceSelections.length > 0) {
                    const firstSource = response.bootSourceSelections[0];
                    setFormData(prev => ({
                        ...prev,
                        os: firstSource.Release
                    }));
                }
            }
        } catch (error) {
            console.error('Error fetching boot sources:', error);
            setNotification({
                open: true,
                message: 'Failed to fetch boot sources',
                severity: 'error'
            });
        } finally {
            setLoading(false);
        }
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const { name, value } = e.target;

        if (name === 'maasUrl') {
            const isValid = maasUrlRegex.test(value);
            setUrlError(value && !isValid ? 'URL must be in format: http(s)://hostname/MAAS' : '');
        }

        setFormData(prev => ({
            ...prev,
            [name]: value
        }));
    };

    const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, checked } = e.target;
        setFormData(prev => ({
            ...prev,
            [name]: checked
        }));
    };

    const handleSelectChange = (e: SelectChangeEvent<string>) => {
        const { name, value } = e.target;
        setFormData(prev => ({
            ...prev,
            [name]: value
        }));
    };

    const handleCloseNotification = () => {
        setNotification(prev => ({
            ...prev,
            open: false
        }));
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        if (urlError) {
            setNotification({
                open: true,
                message: 'Please fix the MAAS URL format',
                severity: 'error'
            });
            return;
        }

        setSubmitting(true);

        try {
            const secretName = 'user-data-secret';

            const existingConfigs = await getBMConfigList(formData.namespace);

            if (existingConfigs && existingConfigs.length > 0) {
                for (const config of existingConfigs) {
                    const fullConfig = await getBMConfig(config.metadata.name, formData.namespace);

                    if (fullConfig &&
                        fullConfig.spec &&
                        fullConfig.spec.userDataSecretRef &&
                        fullConfig.spec.userDataSecretRef.name) {

                        try {
                            const secretName = fullConfig.spec.userDataSecretRef.name;
                            await deleteSecret(secretName, formData.namespace);
                        } catch (secretError) {
                            console.warn('Could not delete associated secret:', secretError);
                        }
                    }

                    await deleteBMConfig(config.metadata.name, formData.namespace);
                }
            }

            await createBmconfigSecret(
                secretName,
                formData.cloudInit,
                formData.namespace
            );

            await createBMConfigWithSecret(
                formData.configName,
                'maas',
                formData.maasUrl,
                formData.apiKey,
                secretName,
                formData.namespace,
                formData.insecure,
                formData.os
            );

            setNotification({
                open: true,
                message: 'MaasConfig saved successfully',
                severity: 'success'
            });
        } catch (error) {
            console.error('Error submitting form:', error);
            reportError(error as Error, {
                context: 'maas-config-submission',
                metadata: {
                    configName: formData.configName,
                    maasUrl: formData.maasUrl,
                    os: formData.os,
                    namespace: formData.namespace,
                    action: 'create-maas-config'
                }
            });
            setNotification({
                open: true,
                message: 'Failed to save MaasConfig',
                severity: 'error'
            });
        } finally {
            setSubmitting(false);
        }
    };

    const handleCancel = () => {
        // Reset form or navigate away
    };

    const handleResetCloudInit = () => {
        setFormData(prev => ({
            ...prev,
            cloudInit: defaultCloudInit
        }));
    };

    return (
        <StyledPaper elevation={0}>
            {initialLoading ? (
                <Box display="flex" justifyContent="center" alignItems="center" height="400px">
                    <CircularProgress />
                    <Typography variant="body1" sx={{ ml: 2 }}>
                        Loading existing configuration...
                    </Typography>
                </Box>
            ) : (
                <Box component="form" onSubmit={handleSubmit} sx={{ display: 'grid' }}>
                    <Section sx={{ mb: 2 }}>
                        <Grid container columnSpacing={4} rowSpacing={2}>
                            {/* First row - MAAS URL and Insecure*/}
                            <Grid item xs={12} md={6}>
                                <FormField>
                                    <Typography variant="subtitle2" gutterBottom>MAAS URL</Typography>
                                    <TextField
                                        fullWidth
                                        name="maasUrl"
                                        value={formData.maasUrl}
                                        onChange={handleChange}
                                        size="small"
                                        variant="outlined"
                                        placeholder="http://10.9.4.234:5240/MAAS"
                                        error={!!urlError}
                                        helperText={urlError}
                                    />
                                </FormField>
                            </Grid>
                            <Grid item xs={12} md={2}>
                                <FormField>
                                    <Typography variant="subtitle2" gutterBottom>Insecure</Typography>
                                    <Switch
                                        name="insecure"
                                        checked={formData.insecure}
                                        onChange={handleSwitchChange}
                                        color="primary"
                                    />
                                </FormField>
                            </Grid>
                            <Grid item md={4} /> {/* Spacer */}

                            {/* Second row - API Key with info icon */}
                            <Grid item xs={12} md={6}>
                                <FormField>
                                    <Typography variant="subtitle2" gutterBottom sx={{ display: 'flex', alignItems: 'center' }}>
                                        API Key
                                        <Tooltip title="The API key for your MAAS account. You can generate this from the MAAS UI under 'API Keys'." arrow>
                                            <InfoIcon fontSize="small" sx={{ ml: 1, opacity: 0.7 }} />
                                        </Tooltip>
                                    </Typography>
                                    <TextField
                                        fullWidth
                                        name="apiKey"
                                        value={formData.apiKey}
                                        onChange={handleChange}
                                        size="small"
                                        variant="outlined"
                                        type="password"
                                    />
                                </FormField>
                            </Grid>

                            <Grid item xs={12} md={6}>
                                <FormField>
                                    <Typography variant="subtitle2" gutterBottom sx={{ display: 'flex', alignItems: 'center' }}>
                                        OS
                                        <Tooltip title="Ubuntu Jammy is only supported for now." arrow>
                                            <InfoIcon fontSize="small" sx={{ ml: 1, opacity: 0.7 }} />
                                        </Tooltip>
                                    </Typography>
                                    {loading ? (
                                        <Box display="flex" alignItems="center">
                                            <CircularProgress size={20} sx={{ mr: 1 }} />
                                            <Typography variant="body2" color="text.secondary">
                                                Loading OS options...
                                            </Typography>
                                        </Box>
                                    ) : (
                                        <FormControl fullWidth size="small">
                                            <Select
                                                name="os"
                                                value={formData.os}
                                                onChange={handleSelectChange}
                                                displayEmpty
                                                disabled={bootSources?.length === 0}
                                            >
                                                <MenuItem value="" disabled>Select an OS</MenuItem>
                                                {bootSources?.map((source) => (
                                                    <MenuItem
                                                        key={`${source.OS} (${source.Release})`}
                                                        value={source.Release}
                                                        disabled={!(source.OS === 'ubuntu' && source.Release === 'jammy')}
                                                    >
                                                        {`${source.OS} (${source.Release})`}
                                                    </MenuItem>
                                                ))}
                                            </Select>
                                        </FormControl>
                                    )}
                                </FormField>
                            </Grid>
                        </Grid>
                    </Section>

                    <Section sx={{ mb: 2 }}>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                            <Typography variant="subtitle2">Cloud-init (YAML):</Typography>
                            <Button
                                variant="outlined"
                                size="small"
                                onClick={handleResetCloudInit}
                                startIcon={<RefreshIcon fontSize="small" />}
                            >
                                Reset to Default
                            </Button>
                        </Box>
                        <CodeMirrorContainer>
                            <CodeMirror
                                value={formData.cloudInit}
                                height="300px"
                                extensions={extensions}
                                theme={isDarkMode ? 'dark' : 'light'}
                                onChange={(value) => {
                                    setFormData(prev => ({
                                        ...prev,
                                        cloudInit: value
                                    }));
                                }}
                                basicSetup={{
                                    lineNumbers: true,
                                    highlightActiveLine: true,
                                    highlightSelectionMatches: true,
                                    syntaxHighlighting: true,
                                }}
                            />
                        </CodeMirrorContainer>
                    </Section>

                    <Footer>
                        <Button variant="outlined" onClick={handleCancel}>
                            Cancel
                        </Button>
                        <Button
                            variant="contained"
                            type="submit"
                            color="primary"
                            disabled={submitting || !formData.os || !!urlError}
                            startIcon={submitting ? <CircularProgress size={20} color="inherit" /> : null}
                        >
                            {submitting ? 'Saving...' : 'Save'}
                        </Button>
                    </Footer>
                </Box>
            )}

            <Snackbar
                open={notification.open}
                autoHideDuration={6000}
                onClose={handleCloseNotification}
            >
                <Alert
                    onClose={handleCloseNotification}
                    severity={notification.severity}
                    sx={{ width: '100%' }}
                >
                    {notification.message}
                </Alert>
            </Snackbar>
        </StyledPaper>
    );
} 