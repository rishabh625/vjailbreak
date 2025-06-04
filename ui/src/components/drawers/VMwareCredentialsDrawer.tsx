import { Box } from "@mui/material";
import { useState, useCallback } from "react";
import { StyledDrawer, DrawerContent } from "src/components/forms/StyledDrawer";
import Header from "src/components/forms/Header";
import Footer from "src/components/forms/Footer";
import { createVMwareCredsWithSecretFlow, deleteVMwareCredsWithSecretFlow } from "src/api/helpers";
import axios from "axios";
import { useVmwareCredentialsQuery } from "src/hooks/api/useVmwareCredentialsQuery";
import { TextField, FormControl, InputAdornment, IconButton, FormLabel, CircularProgress, FormControlLabel, Switch } from "@mui/material";
import { Visibility, VisibilityOff } from "@mui/icons-material";
import CheckIcon from "@mui/icons-material/Check";
import { isValidName } from "src/utils";
import { getVmwareCredentials } from "src/api/vmware-creds/vmwareCreds";
import { useInterval } from "src/hooks/useInterval";
import { THREE_SECONDS } from "src/constants";
import { useKeyboardSubmit } from "src/hooks/ui/useKeyboardSubmit";

interface VMwareCredentialsDrawerProps {
    open: boolean;
    onClose: () => void;
}

export default function VMwareCredentialsDrawer({
    open,
    onClose,
}: VMwareCredentialsDrawerProps) {
    const [validatingVmwareCreds, setValidatingVmwareCreds] = useState(false);
    const [vmwareCredsValidated, setVmwareCredsValidated] = useState<boolean | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [credNameError, setCredNameError] = useState<string | null>(null);
    const [submitting, setSubmitting] = useState(false);
    const [showPassword, setShowPassword] = useState(false);
    const [createdCredentialName, setCreatedCredentialName] = useState<string | null>(null);

    const { refetch: refetchVmwareCreds } = useVmwareCredentialsQuery();

    const closeDrawer = useCallback(() => {
        // Check if we have a created credential that hasn't been fully validated (succeeded)
        if (createdCredentialName) {
            console.log(`Cleaning up VMware credential on drawer close: ${createdCredentialName}`);
            try {
                deleteVMwareCredsWithSecretFlow(createdCredentialName)
                    .then(() => console.log(`Cancelled credential ${createdCredentialName} deleted successfully`))
                    .catch(err => console.error(`Error deleting cancelled credential: ${createdCredentialName}`, err));
            } catch (err) {
                console.error(`Error initiating deletion of cancelled credential: ${createdCredentialName}`, err);
            }
        }

        // Reset state
        setFormValues({
            credentialName: "",
            vcenterHost: "",
            datacenter: "",
            username: "",
            password: "",
            insecure: false,
        });
        setCreatedCredentialName(null);
        setValidatingVmwareCreds(false);
        setVmwareCredsValidated(null);
        setError(null);
        setCredNameError(null);
        setSubmitting(false);
        setShowPassword(false);

        onClose();
    }, [createdCredentialName, onClose]);

    // Track form values
    const [formValues, setFormValues] = useState({
        credentialName: "",
        vcenterHost: "",
        datacenter: "",
        username: "",
        password: "",
        insecure: false,
    });

    const isValidCredentialName = isValidName(formValues.credentialName);

    // Define a clear polling condition similar to MigrationForm
    const shouldPollVmwareCreds =
        !!createdCredentialName &&
        validatingVmwareCreds;

    const handleClickShowPassword = () => {
        setShowPassword(!showPassword);
    };

    const handleMouseDownPassword = (event: React.MouseEvent<HTMLButtonElement>) => {
        event.preventDefault();
    };

    const handleFormChange = (field: string) => (event: React.ChangeEvent<HTMLInputElement>) => {
        const value = field === 'insecure' ? event.target.checked : event.target.value;

        setFormValues(prev => ({
            ...prev,
            [field]: value
        }));

        setVmwareCredsValidated(null);
        setError(null);

        if (field === 'credentialName') {
            if (!isValidName(event.target.value)) {
                setCredNameError("Credential name must start with a letter or number, followed by letters, numbers or hyphens, with a maximum length of 253 characters");
            } else {
                setCredNameError(null);
            }
        }
    };

    const handleValidationStatus = (status: string, message?: string) => {
        if (status === "Succeeded") {
            setVmwareCredsValidated(true);
            setValidatingVmwareCreds(false);
            // Close the drawer after a short delay to show success state
            setTimeout(() => {
                refetchVmwareCreds();
                onClose();
            }, 1500);
        } else if (status === "Failed") {
            setVmwareCredsValidated(false);
            setValidatingVmwareCreds(false);
            setError(message || "Validation failed");

            // Try to delete the failed credential to clean up
            if (createdCredentialName) {
                try {
                    deleteVMwareCredsWithSecretFlow(createdCredentialName)
                        .then(() => console.log(`Failed credential ${createdCredentialName} deleted`))
                        .catch((deleteErr) => console.error(`Error deleting failed credential: ${createdCredentialName}`, deleteErr));
                } catch (deleteErr) {
                    console.error(`Error deleting failed credential: ${createdCredentialName}`, deleteErr);
                }
            }
        }
        setSubmitting(false);
    };

    useInterval(
        async () => {
            try {
                const response = await getVmwareCredentials(createdCredentialName);
                if (response?.status?.vmwareValidationStatus) {
                    handleValidationStatus(
                        response.status.vmwareValidationStatus,
                        response.status.vmwareValidationMessage
                    );
                }
            } catch (err) {
                console.error("Error validating VMware credentials", err);
                setError("Error validating VMware credentials");
                setValidatingVmwareCreds(false);
                setSubmitting(false);
            }
        },
        THREE_SECONDS,
        shouldPollVmwareCreds
    );

    const handleSubmit = useCallback(async () => {
        if (!formValues.credentialName || !formValues.vcenterHost || !formValues.datacenter || !formValues.username || !formValues.password) {
            setError("Please fill in all required fields");
            return;
        }

        if (!isValidCredentialName) {
            setError("Please provide a valid credential name");
            return;
        }

        setSubmitting(true);
        setValidatingVmwareCreds(true);

        try {
            const credentialData = {
                VCENTER_HOST: formValues.vcenterHost,
                VCENTER_DATACENTER: formValues.datacenter,
                VCENTER_USERNAME: formValues.username,
                VCENTER_PASSWORD: formValues.password,
                ...(formValues.insecure && { VCENTER_INSECURE: true }),
            };

            const response = await createVMwareCredsWithSecretFlow(
                formValues.credentialName,
                credentialData
            );

            setCreatedCredentialName(response.metadata.name);
        } catch (error) {
            console.error("Error creating VMware credentials:", error);
            setVmwareCredsValidated(false);
            setValidatingVmwareCreds(false);
            setError(
                "Error creating VMware credentials: " + (axios.isAxiosError(error) ? error?.response?.data?.message : error)
            );
            setSubmitting(false);
        }
    }, [formValues, isValidCredentialName]);

    useKeyboardSubmit({
        open,
        isSubmitDisabled: submitting || validatingVmwareCreds || !isValidCredentialName ||
            !formValues.vcenterHost || !formValues.datacenter || !formValues.username || !formValues.password,
        onSubmit: handleSubmit,
        onClose: closeDrawer
    });

    return (
        <StyledDrawer
            anchor="right"
            open={open}
            onClose={closeDrawer}
        >
            <Header title="Add VMware Credentials" />
            <DrawerContent>
                <Box sx={{ display: "grid", gap: 3 }}>
                    <FormControl fullWidth error={!!error || !!credNameError} required>
                        <TextField
                            id="credentialName"
                            label="Enter VMware Credential Name"
                            variant="outlined"
                            value={formValues.credentialName}
                            onChange={handleFormChange('credentialName')}
                            error={!!error || !!credNameError}
                            helperText={
                                credNameError || (error && !credNameError ? error : "")
                            }
                            required
                            fullWidth
                            size="small"
                            sx={{ mb: 2 }}
                        />

                        <TextField
                            id="vcenterHost"
                            label="vCenter Server"
                            variant="outlined"
                            value={formValues.vcenterHost}
                            onChange={handleFormChange('vcenterHost')}
                            error={!!error}
                            required
                            fullWidth
                            size="small"
                            sx={{ mb: 2 }}
                        />

                        <TextField
                            id="datacenter"
                            label="Datacenter Name"
                            variant="outlined"
                            value={formValues.datacenter}
                            onChange={handleFormChange('datacenter')}
                            error={!!error}
                            required
                            fullWidth
                            size="small"
                            sx={{ mb: 2 }}
                        />

                        <TextField
                            id="username"
                            label="Username"
                            variant="outlined"
                            value={formValues.username}
                            onChange={handleFormChange('username')}
                            error={!!error}
                            required
                            fullWidth
                            size="small"
                            sx={{ mb: 2 }}
                        />

                        <TextField
                            id="password"
                            label="Password"
                            type={showPassword ? "text" : "password"}
                            variant="outlined"
                            value={formValues.password}
                            onChange={handleFormChange('password')}
                            error={!!error}
                            required
                            fullWidth
                            size="small"
                            InputProps={{
                                endAdornment: (
                                    <InputAdornment position="end">
                                        <IconButton
                                            onClick={handleClickShowPassword}
                                            onMouseDown={handleMouseDownPassword}
                                            edge="end"
                                            size="small"
                                        >
                                            {showPassword ? <VisibilityOff /> : <Visibility />}
                                        </IconButton>
                                    </InputAdornment>
                                ),
                            }}
                        />

                        {/* Insecure Connection Toggle */}
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={formValues.insecure}
                                    onChange={handleFormChange('insecure')}
                                    name="insecure"
                                    size="small"
                                />
                            }
                            label="Allow insecure connection (skip SSL verification)"
                            sx={{ mt: 1, mb: 1 }}
                        />

                        {/* VMware Validation Status */}
                        <Box sx={{ display: "flex", gap: 2, mt: 2, alignItems: "center" }}>
                            {validatingVmwareCreds && (
                                <>
                                    <CircularProgress size={24} />
                                    <FormLabel>
                                        Validating  VMware credentials...
                                    </FormLabel>
                                </>
                            )}
                            {vmwareCredsValidated === true && formValues.credentialName && (
                                <>
                                    <CheckIcon color="success" fontSize="small" />
                                    <FormLabel>VMware credentials created</FormLabel>
                                </>
                            )}
                            {error && !credNameError && (
                                <FormLabel sx={{ fontSize: "12px" }} color="error">
                                    {error}
                                </FormLabel>
                            )}
                        </Box>
                    </FormControl>
                </Box>
            </DrawerContent>
            <Footer
                submitButtonLabel={validatingVmwareCreds ? "Validating..." : "Create Credentials"}
                onClose={closeDrawer}
                onSubmit={handleSubmit}
                disableSubmit={submitting || validatingVmwareCreds || !isValidCredentialName || !formValues.vcenterHost || !formValues.datacenter || !formValues.username || !formValues.password}
                submitting={submitting}
            />
        </StyledDrawer>
    );
} 