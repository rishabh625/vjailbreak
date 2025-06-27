import {
    styled,
    Typography,
    Box,
    FormControl,
    Select,
    MenuItem,
    ListSubheader,
    CircularProgress,
} from "@mui/material"
import Step from "../../components/forms/Step"
import vmwareLogo from "src/assets/vmware.jpeg"
import { useClusterData } from "./useClusterData"

import "@cds/core/icon/register.js"
import { ClarityIcons, buildingIcon, clusterIcon } from "@cds/core/icon"

ClarityIcons.addIcons(buildingIcon, clusterIcon)

const VMwareLogoImg = styled('img')({
    width: 24,
    height: 24,
    marginRight: 8,
    objectFit: 'contain'
});

const CdsIconWrapper = styled('div')({
    marginRight: 8,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center'
});

const ClusterSelectionStepContainer = styled("div")(({ theme }) => ({
    display: "grid",
    gridGap: theme.spacing(1),
}))

const SideBySideContainer = styled(Box)(({ theme }) => ({
    display: "grid",
    gridTemplateColumns: "1fr 1fr",
    gap: theme.spacing(3),
    marginLeft: theme.spacing(6),
}))

interface SourceDestinationClusterSelectionProps {
    onChange: (id: string) => (value: unknown) => void
    errors: { [fieldId: string]: string }
    vmwareCluster?: string
    pcdCluster?: string
    stepNumber?: string
    stepLabel?: string
    onVmwareClusterChange?: (value: string) => void
    onPcdClusterChange?: (value: string) => void
    loadingVMware?: boolean
    loadingPCD?: boolean
}

export default function SourceDestinationClusterSelection({
    onChange,
    errors,
    vmwareCluster = "",
    pcdCluster = "",
    stepNumber = "1",
    stepLabel = "Source and Destination Clusters",
    onVmwareClusterChange,
    onPcdClusterChange,
    loadingVMware: externalLoadingVMware,
    loadingPCD: externalLoadingPCD,
}: SourceDestinationClusterSelectionProps) {
    // Use the centralized cluster data hook
    const {
        sourceData,
        pcdData,
        loadingVMware: hookLoadingVMware,
        loadingPCD: hookLoadingPCD
    } = useClusterData();

    // Use external loading states if provided, otherwise use hook loading states
    const loadingVMware = externalLoadingVMware !== undefined ? externalLoadingVMware : hookLoadingVMware;
    const loadingPCD = externalLoadingPCD !== undefined ? externalLoadingPCD : hookLoadingPCD;

    // Use alternative prop names if provided
    const currentVmwareCluster = vmwareCluster;
    const currentPcdCluster = pcdCluster;

    const handleVMwareClusterChange = (event) => {
        const value = event.target.value;
        onChange("vmwareCluster")(value);

        if (value) {
            const parts = value.split(":");
            const credName = parts[0];

            onChange("vmwareCreds")({
                existingCredName: credName,
            });
        } else {
            onChange("vmwareCreds")({});
        }
    };

    const handlePcdClusterChange = (event) => {
        const value = event.target.value;
        onChange("pcdCluster")(value);

        if (value) {
            const selectedPCD = pcdData.find(p => p.id === value);
            if (selectedPCD) {
                onChange("openstackCreds")({
                    existingCredName: selectedPCD.openstackCredName,
                });
            }
        } else {
            onChange("openstackCreds")({});
        }
    };

    const handleVMwareClusterChangeWrapper = (event) => {
        const value = event.target.value;
        if (onVmwareClusterChange) {
            onVmwareClusterChange(value);
        } else {
            handleVMwareClusterChange(event);
        }
    };

    const handlePcdClusterChangeWrapper = (event) => {
        const value = event.target.value;
        if (onPcdClusterChange) {
            onPcdClusterChange(value);
        } else {
            handlePcdClusterChange(event);
        }
    };

    return (
        <ClusterSelectionStepContainer>
            <Step stepNumber={stepNumber} label={stepLabel} />
            <SideBySideContainer>
                <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: "500" }}>VMware Source Cluster</Typography>
                    <FormControl fullWidth variant="outlined" size="small">
                        <Select
                            value={currentVmwareCluster}
                            onChange={handleVMwareClusterChangeWrapper}
                            displayEmpty
                            disabled={loadingVMware}
                            error={!!errors["vmwareCluster"]}
                            renderValue={(selected) => {
                                if (!selected) return <em>Select VMware Cluster</em>;
                                const parts = selected.split(":");
                                const credName = parts[0];

                                const sourceItem = sourceData.find(item => item.credName === credName);
                                const vcenterName = sourceItem?.vcenterName || credName;
                                const cluster = sourceItem?.clusters.find(c => c.id === selected);
                                return `${vcenterName} - ${sourceItem?.datacenter || ""} - ${cluster?.name || ""}`;
                            }}
                            endAdornment={loadingVMware ? <CircularProgress size={25} sx={{ mr: 3, display: "flex", alignItems: "center", justifyContent: "center" }} /> : null}
                            MenuProps={{
                                PaperProps: {
                                    style: {
                                        maxHeight: 300
                                    }
                                }
                            }}
                        >
                            <MenuItem value="" disabled><em>Select VMware Cluster</em></MenuItem>

                            {sourceData.length === 0 ? (
                                <MenuItem disabled>No clusters found</MenuItem>
                            ) : (
                                Object.entries(
                                    sourceData.reduce((acc, item) => {
                                        if (!acc[item.vcenterName]) {
                                            acc[item.vcenterName] = {
                                                credName: item.credName,
                                                datacenters: {}
                                            };
                                        }
                                        acc[item.vcenterName].datacenters[item.datacenter] = item.clusters;
                                        return acc;
                                    }, {} as Record<string, { credName: string, datacenters: Record<string, { id: string; name: string }[]> }>)
                                ).map(([vcenterName, { credName, datacenters }]) => [
                                    <ListSubheader key={vcenterName} sx={{ fontWeight: 700 }}>
                                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                                            <VMwareLogoImg src={vmwareLogo} alt="VMware" />
                                            {vcenterName}
                                        </Box>
                                    </ListSubheader>,
                                    ...Object.entries(datacenters).map(([datacenterName, clusters]) => [
                                        <ListSubheader key={`${credName}-${datacenterName}`} sx={{ fontWeight: 600, pl: 4 }}>
                                            <Box sx={{ display: 'flex', alignItems: 'center' }}>
                                                <CdsIconWrapper>
                                                    {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                                                    {/* @ts-ignore */}
                                                    <cds-icon shape="building" size="md" solid ></cds-icon>
                                                </CdsIconWrapper>
                                                {datacenterName}
                                            </Box>
                                        </ListSubheader>,
                                        ...clusters.map((cluster) => (
                                            <MenuItem
                                                key={cluster.id}
                                                value={cluster.id}
                                                sx={{ pl: 7 }}
                                            >
                                                <Box sx={{ display: 'flex', alignItems: 'center' }}>
                                                    <CdsIconWrapper>
                                                        {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                                                        {/* @ts-ignore */}
                                                        <cds-icon shape="cluster" size="md" ></cds-icon>
                                                    </CdsIconWrapper>
                                                    {cluster.name}
                                                </Box>
                                            </MenuItem>
                                        ))
                                    ])
                                ]).flat()
                            )}
                        </Select>
                    </FormControl>
                    {errors["vmwareCluster"] && (
                        <Typography variant="caption" color="error" sx={{ mt: 1, display: 'block' }}>
                            {errors["vmwareCluster"]}
                        </Typography>
                    )}
                </Box>

                <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: "500" }}>PCD Destination Cluster</Typography>
                    <FormControl fullWidth variant="outlined" size="small">
                        <Select
                            value={currentPcdCluster}
                            onChange={handlePcdClusterChangeWrapper}
                            displayEmpty
                            disabled={loadingPCD}
                            error={!!errors["pcdCluster"]}
                            renderValue={(selected) => {
                                if (!selected) return <em>Select PCD Cluster</em>;
                                const pcd = pcdData.find(p => p.id === selected);
                                return pcd?.name || selected;
                            }}
                            endAdornment={loadingPCD ? <CircularProgress size={25} sx={{ mr: 3, display: "flex", alignItems: "center", justifyContent: "center" }} /> : null}
                            MenuProps={{
                                PaperProps: {
                                    style: {
                                        maxHeight: 300
                                    }
                                }
                            }}
                        >
                            <MenuItem value="" disabled><em>Select PCD Cluster</em></MenuItem>

                            {pcdData.length === 0 ? (
                                <MenuItem disabled>No PCD clusters found</MenuItem>
                            ) : (
                                pcdData.map((pcd) => (
                                    <MenuItem
                                        key={pcd.id}
                                        value={pcd.id}
                                    >
                                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                                            <CdsIconWrapper>
                                                {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment */}
                                                {/* @ts-ignore */}
                                                <cds-icon shape="cluster" size="md"></cds-icon>
                                            </CdsIconWrapper>
                                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                                <Typography variant="body1">{pcd.name}</Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    Credential: {pcd.openstackCredName} | Tenant: {pcd.tenantName}

                                                </Typography>
                                            </Box>
                                        </Box>
                                    </MenuItem>
                                ))
                            )}
                        </Select>
                    </FormControl>
                    {errors["pcdCluster"] && (
                        <Typography variant="caption" color="error" sx={{ mt: 1, display: 'block' }}>
                            {errors["pcdCluster"]}
                        </Typography>
                    )}
                </Box>
            </SideBySideContainer>
        </ClusterSelectionStepContainer>
    )
} 