import React, { useState, useEffect, useRef, useCallback } from "react";
import {
    Box,
    Typography,
    IconButton,
    Switch,
    FormControlLabel,
    CircularProgress,
    Alert,
    Paper,
    ToggleButton,
    ToggleButtonGroup,
} from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import { StyledDrawer, DrawerContent } from "src/components/forms/StyledDrawer";
import { usePodLogs } from "src/hooks/usePodLogs";
import { useDeploymentLogs } from "src/hooks/useDeploymentLogs";

interface LogsDrawerProps {
    open: boolean;
    onClose: () => void;
    podName: string;
    namespace: string;
    migrationName?: string;
}

export default function LogsDrawer({
    open,
    onClose,
    podName,
    namespace,
    migrationName,
}: LogsDrawerProps) {
    const [follow, setFollow] = useState(true);
    const [logSource, setLogSource] = useState<'pod' | 'controller'>('pod');
    const [isTransitioning, setIsTransitioning] = useState(false);
    const logsEndRef = useRef<HTMLDivElement>(null);
    const logsContainerRef = useRef<HTMLDivElement>(null);

    const { logs, isLoading, error, reconnect } = usePodLogs({
        jobName: podName,
        namespace,
        enabled: open && logSource === 'pod',
    });

    const controllerLogs = useDeploymentLogs({
        deploymentName: 'migration-controller-manager',
        namespace: 'migration-system',
        labelSelector: 'control-plane=controller-manager',
        enabled: open && logSource === 'controller'
    });

    // Get current logs and states based on log source
    const currentLogs = logSource === 'pod' ? logs : controllerLogs.logs;
    const currentIsLoading = logSource === 'pod' ? isLoading : controllerLogs.isLoading;
    const currentError = logSource === 'pod' ? error : controllerLogs.error;
    const currentReconnect = logSource === 'pod' ? reconnect : controllerLogs.reconnect;

    // Auto-scroll to bottom when new logs arrive and follow is enabled
    useEffect(() => {
        if (follow && logsEndRef.current && !isTransitioning) {
            logsEndRef.current.scrollIntoView({ behavior: "smooth" });
        }
    }, [currentLogs, follow, isTransitioning]);

    const handleFollowToggle = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
        setFollow(event.target.checked);
    }, []);

    const handleLogSourceChange = useCallback((
        _event: React.MouseEvent<HTMLElement>,
        newLogSource: 'pod' | 'controller' | null
    ) => {
        if (newLogSource !== null && newLogSource !== logSource) {
            setIsTransitioning(true);
            setLogSource(newLogSource);
            
            // Reset transition state after a brief delay to show loading state
            setTimeout(() => {
                setIsTransitioning(false);
            }, 100);
        }
    }, [logSource]);

    const handleClose = useCallback(() => {
        setFollow(true); // Reset follow state when closing
        setLogSource('pod'); // Reset log source when closing
        setIsTransitioning(false); // Reset transition state when closing
        onClose();
    }, [onClose]);

    return (
        <StyledDrawer
            anchor="right"
            open={open}
            onClose={handleClose}
        >
            {/* Header */}
            <Box
                sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 3,
                    py: 2,
                    borderBottom: 1,
                    borderColor: "divider",
                    backgroundColor: "background.paper",
                }}
            >
                <Box>
                    <Typography variant="h6" component="h2">
                        {logSource === 'pod' ? 'Migration Pod Logs' : 'Controller Logs'}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {logSource === 'pod' 
                            ? `${podName} (${namespace})` 
                            : `migration-controller-manager (migration-system)`
                        }
                        {migrationName && (
                            <Typography component="span" variant="body2" color="text.secondary" sx={{ ml: 1 }}>
                                • Migration: {migrationName}
                            </Typography>
                        )}
                    </Typography>
                </Box>
                <IconButton
                    onClick={handleClose}
                    aria-label="close logs drawer"
                    size="small"
                >
                    <CloseIcon />
                </IconButton>
            </Box>

            <DrawerContent>
                <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
                    {/* Log Source Toggle */}
                    <Box
                        sx={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            mb: 2,
                            pb: 1,
                            borderBottom: 1,
                            borderColor: "divider",
                        }}
                    >
                        <ToggleButtonGroup
                            value={logSource}
                            exclusive
                            onChange={handleLogSourceChange}
                            aria-label="log source"
                            size="small"
                        >
                            <ToggleButton value="pod" aria-label="migration pod logs">
                                Migration Pod
                            </ToggleButton>
                            <ToggleButton value="controller" aria-label="controller logs">
                                Controller
                            </ToggleButton>
                        </ToggleButtonGroup>
                    </Box>

                    {/* Controls */}
                    <Box
                        sx={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "space-between",
                            mb: 2,
                            pb: 1,
                            borderBottom: 1,
                            borderColor: "divider",
                        }}
                    >
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={follow}
                                    onChange={handleFollowToggle}
                                    name="follow"
                                    size="small"
                                />
                            }
                            label="Follow logs"
                        />
                        {currentError && (
                            <IconButton
                                onClick={currentReconnect}
                                size="small"
                                color="primary"
                                title="Reconnect"
                            >
                                <Typography variant="caption">Retry</Typography>
                            </IconButton>
                        )}
                    </Box>

                    {/* Loading State */}
                    {(currentIsLoading || isTransitioning) && (
                        <Box
                            sx={{
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                                py: 4,
                            }}
                        >
                            <CircularProgress size={24} sx={{ mr: 2 }} />
                            <Typography variant="body2" color="text.secondary">
                                {isTransitioning 
                                    ? `Switching to ${logSource === 'pod' ? 'pod' : 'controller'} logs...`
                                    : `Connecting to ${logSource === 'pod' ? 'pod' : 'controller'} log stream...`
                                }
                            </Typography>
                        </Box>
                    )}

                    {/* Error State */}
                    {currentError && (
                        <Alert
                            severity="error"
                            sx={{ mb: 2 }}
                            action={
                                <IconButton
                                    color="inherit"
                                    size="small"
                                    onClick={currentReconnect}
                                >
                                    <Typography variant="caption">Retry</Typography>
                                </IconButton>
                            }
                        >
                            Failed to connect to {logSource === 'pod' ? 'pod' : 'controller'} log stream: {currentError}
                        </Alert>
                    )}

                    {/* Logs Display */}
                    <Paper
                        variant="outlined"
                        sx={{
                            flex: 1,
                            overflow: "hidden",
                            display: "flex",
                            flexDirection: "column",
                        }}
                    >
                        <Box
                            ref={logsContainerRef}
                            sx={{
                                flex: 1,
                                overflow: "auto",
                                p: 2,
                                backgroundColor: "#1e1e1e",
                                color: "#ffffff",
                                fontFamily: "monospace",
                                fontSize: "0.875rem",
                                lineHeight: 1.4,
                                whiteSpace: "pre-wrap",
                                wordBreak: "break-word",
                            }}
                        >
                            {currentLogs.length === 0 && !currentIsLoading && !currentError && !isTransitioning && (
                                <Typography
                                    variant="body2"
                                    color="text.secondary"
                                    sx={{ fontFamily: "monospace" }}
                                >
                                    No logs available
                                </Typography>
                            )}
                            {currentLogs.map((log, index) => (
                                <Box
                                    key={index}
                                    sx={{
                                        borderBottom: index < currentLogs.length - 1 ? "1px solid #333" : "none",
                                        py: 0.5,
                                    }}
                                >
                                    {log}
                                </Box>
                            ))}
                            <div ref={logsEndRef} />
                        </Box>
                    </Paper>
                </Box>
            </DrawerContent>
        </StyledDrawer>
    );
}
