import { Paper, styled, Tab, Tabs, Box } from "@mui/material"
import { useEffect, useState } from "react"
import { useNavigate, useLocation } from "react-router-dom"
import { FIVE_SECONDS, THIRTY_SECONDS } from "src/constants"
import { useMigrationsQuery } from "src/hooks/api/useMigrationsQuery"
import { deleteMigration } from "src/api/migrations/migrations"
import { useQueryClient } from "@tanstack/react-query"
import { MIGRATIONS_QUERY_KEY } from "src/hooks/api/useMigrationsQuery"
import ConfirmationDialog from "src/components/dialogs/ConfirmationDialog"
import { getMigrationPlan, patchMigrationPlan } from "src/api/migration-plans/migrationPlans"
import { Migration } from "src/api/migrations/model"
import MigrationsTable from "./MigrationsTable"
import { deleteClusterMigration } from "src/api/clustermigrations/clustermigrations"
import { CLUSTER_MIGRATIONS_QUERY_KEY } from "src/hooks/api/useClusterMigrationsQuery"
import NodesTable from "./NodesTable"
import CredentialsTable from "./CredentialsTable"
import BMConfigForm from "./BMConfigForm"
import { ClusterMigration } from "src/api/clustermigrations/model"
import { useESXIMigrationsQuery, ESXI_MIGRATIONS_QUERY_KEY } from "../../hooks/api/useESXIMigrationsQuery"
import { ESXIMigration } from "src/api/esximigrations/model"
import { deleteESXIMigration } from "src/api/esximigrations/esximigrations"
import RollingMigrationsTable from "./RollingMigrationsTable"
import WarningIcon from '@mui/icons-material/Warning';
import { useNodesQuery } from "../../hooks/api/useNodesQuery"
import { useClusterMigrationsQuery } from "../../hooks/api/useClusterMigrationsQuery"


const DashboardContainer = styled("div")({
  display: "flex",
  justifyContent: "center",
  width: "100%",
  height: "100%",
  padding: "60px 20px",
  boxSizing: "border-box"
})

const StyledPaper = styled(Paper)({
  width: "100%",
  "& .MuiDataGrid-virtualScroller": {
    overflowX: "hidden"
  }
})


export default function Dashboard() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [selectedMigrations, setSelectedMigrations] = useState<Migration[]>([])
  const [selectedClusterMigrations, setSelectedClusterMigrations] = useState<ClusterMigration[]>([])
  const [selectedESXIMigrations, setSelectedESXIMigrations] = useState<ESXIMigration[]>([])
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteType, setDeleteType] = useState<'migration' | 'clusterMigration' | 'esxiMigration'>('migration')
  const [activeTab, setActiveTab] = useState(0)

  const { data: migrations, refetch: refetchMigrations } = useMigrationsQuery(undefined, {
    refetchInterval: (query) => {
      const migrations = query?.state?.data || []
      const hasPendingMigration = !!migrations.find(
        (m) => m.status === undefined
      )
      return hasPendingMigration ? FIVE_SECONDS : THIRTY_SECONDS
    },
    staleTime: 0,
    refetchOnMount: true
  })

  const { data: clusterMigrations, refetch: refetchClusterMigrations } = useClusterMigrationsQuery({
    queryKey: CLUSTER_MIGRATIONS_QUERY_KEY,
    refetchInterval: THIRTY_SECONDS,
    staleTime: 0,
    refetchOnMount: true
  })

  const { data: esxiMigrations } = useESXIMigrationsQuery({
    queryKey: ESXI_MIGRATIONS_QUERY_KEY,
    refetchInterval: THIRTY_SECONDS,
    staleTime: 0,
    refetchOnMount: true
  })

  const handleDeleteClick = (migrationName: string) => {
    const migration = migrations?.find(m => m.metadata.name === migrationName)
    if (migration) {
      setSelectedMigrations([migration])
      setDeleteType('migration')
      setDeleteDialogOpen(true)
    }
  }

  const handleDeleteClose = () => {
    setDeleteDialogOpen(false)
    setSelectedMigrations([])
    setSelectedClusterMigrations([])
    setSelectedESXIMigrations([])
    setDeleteError(null)
  }

  const handleDeleteSelected = (migrations: Migration[]) => {
    setSelectedMigrations(migrations)
    setDeleteType('migration')
    setDeleteDialogOpen(true)
  }




  const handleDeleteMigration = async (migrations: Migration[]) => {
    // Group VMs by migration plan
    const migrationPlanUpdates = migrations.reduce((acc, migration) => {
      const planId = migration.spec.migrationPlan
      if (!acc[planId]) {
        acc[planId] = {
          vmsToRemove: new Set<string>(),
          migrationsToDelete: new Set<string>()
        }
      }
      acc[planId].vmsToRemove.add(migration.spec.vmName)
      acc[planId].migrationsToDelete.add(migration.metadata.name)
      return acc
    }, {} as Record<string, { vmsToRemove: Set<string>, migrationsToDelete: Set<string> }>)

    // Update each migration plan once
    await Promise.all(
      Object.entries(migrationPlanUpdates).map(async ([planId, { vmsToRemove, migrationsToDelete }]) => {
        const migrationPlan = await getMigrationPlan(planId)
        const updatedVirtualMachines = migrationPlan.spec.virtualMachines?.[0]?.filter(
          vm => !vmsToRemove.has(vm)
        )

        await patchMigrationPlan(planId, {
          spec: {
            virtualMachines: [updatedVirtualMachines]
          }
        })

        // Delete all migrations for this plan
        await Promise.all(
          Array.from(migrationsToDelete).map(migrationName =>
            deleteMigration(migrationName)
          )
        )
      })
    )

    queryClient.invalidateQueries({ queryKey: MIGRATIONS_QUERY_KEY })
    handleDeleteClose()
  }

  const handleDeleteClusterMigration = async (clusterMigrations: ClusterMigration[]) => {
    await Promise.all(
      clusterMigrations.map(migration =>
        deleteClusterMigration(migration.metadata.name)
      )
    )
    queryClient.invalidateQueries({ queryKey: CLUSTER_MIGRATIONS_QUERY_KEY })
    handleDeleteClose()
  }

  const handleDeleteESXIMigration = async (esxiMigrations: ESXIMigration[]) => {
    await Promise.all(
      esxiMigrations.map(migration =>
        deleteESXIMigration(migration.metadata.name)
      )
    )
    queryClient.invalidateQueries({ queryKey: ESXI_MIGRATIONS_QUERY_KEY })
    handleDeleteClose()
  }


  const handleDeleteAction = (): Promise<void> => {
    if (deleteType === 'migration') {
      return handleDeleteMigration(selectedMigrations)
    } else if (deleteType === 'clusterMigration') {
      return handleDeleteClusterMigration(selectedClusterMigrations)
    } else {
      return handleDeleteESXIMigration(selectedESXIMigrations)
    }
  }

  const getDeleteDialogTitle = () => {
    if (deleteType === 'migration') {
      return "Confirm Delete Migration"
    } else if (deleteType === 'clusterMigration') {
      return "Confirm Delete Cluster Conversion"
    } else if (deleteType === 'esxiMigration') {
      return "Confirm Delete ESXi Migration"
    }
    return "Confirm Delete"
  }

  const getDeleteDialogMessage = () => {
    if (deleteType === 'migration') {
      return selectedMigrations.length > 1
        ? "Are you sure you want to delete these migrations?"
        : `Are you sure you want to delete migration "${selectedMigrations[0]?.metadata.name}"?`
    } else if (deleteType === 'clusterMigration') {
      return selectedClusterMigrations.length > 1
        ? "Are you sure you want to delete these cluster conversions?"
        : `Are you sure you want to delete cluster conversion "${selectedClusterMigrations[0]?.metadata.name}"?`
    } else if (deleteType === 'esxiMigration') {
      return selectedESXIMigrations.length > 1
        ? "Are you sure you want to delete these ESXi migrations?"
        : `Are you sure you want to delete ESXi migration "${selectedESXIMigrations[0]?.metadata.name}"?`
    }
    return "Are you sure you want to delete the selected items?"
  }

  const getDeleteDialogItems = () => {
    if (deleteType === 'migration') {
      return selectedMigrations.map(m => ({
        id: m.metadata.name,
        name: m.metadata.name
      }))
    } else if (deleteType === 'clusterMigration') {
      return selectedClusterMigrations.map(m => ({
        id: m.metadata.name,
        name: m.metadata.name
      }))
    } else if (deleteType === 'esxiMigration') {
      return selectedESXIMigrations.map(m => ({
        id: m.metadata.name,
        name: m.metadata.name
      }))
    }
    return []
  }

  const getCustomErrorMessage = (error: Error | string) => {
    const baseMessage = "Failed to delete migrations"
    if (error instanceof Error) {
      return `${baseMessage}: ${error.message}`
    }
    return baseMessage
  }

  const { data: nodes } = useNodesQuery()

  useEffect(() => {
    if (!!migrations && migrations.length === 0 && (!nodes || nodes.length === 0)) {
      navigate("/onboarding")
    }
  }, [migrations, navigate])

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const tabParam = params.get('tab');
    if (tabParam) {
      const tabOrder = {
        'migrations': 0,
        'agents': 1,
        'credentials': 2,
        'clusterconversions': 3,
        'clustermigrations': 3,
        'maasconfig': 4
      };

      if (tabParam in tabOrder) {
        setActiveTab(tabOrder[tabParam]);
        // If using old route, redirect to new route
        if (tabParam === 'clustermigrations') {
          navigate('/dashboard?tab=clusterconversions', { replace: true });
        }
      } else {
        const tabIndex = parseInt(tabParam, 10);
        if (!isNaN(tabIndex) && tabIndex >= 0 && tabIndex <= 4) {
          setActiveTab(tabIndex);
        }
      }
    }
  }, [location, navigate]);

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue)
    const tabNames = [
      'migrations',
      'agents',
      'credentials',
      'clusterconversions',
      'maasconfig'
    ];
    navigate(`/dashboard?tab=${tabNames[newValue]}`, { replace: true });
  }

  return (
    <DashboardContainer>
      <StyledPaper>
        <Tabs
          value={activeTab}
          onChange={handleTabChange}
          sx={{ borderBottom: 1, borderColor: 'divider' }}
        >
          <Tab label="Migrations" />
          <Tab label="Agents" />
          <Tab label="Credentials" />
          <Tab
            label={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Box sx={{
                  color: 'text.secondary',
                  fontSize: '1rem',
                  fontWeight: 300,
                  mr: 2
                }}>
                  |
                </Box>
                Cluster Conversions
              </Box>
            }
          />
          <Tab
            label={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                Maas Config
              </Box>
            }
          />
        </Tabs>

        {activeTab === 0 ? (
          <MigrationsTable
            refetchMigrations={refetchMigrations}
            migrations={migrations || []}
            onDeleteMigration={handleDeleteClick}
            onDeleteSelected={handleDeleteSelected}
          />
        ) : activeTab === 1 ? (
          <NodesTable />
        ) : activeTab === 2 ? (
          <CredentialsTable />
        ) : activeTab === 3 ? (
          <RollingMigrationsTable
            clusterMigrations={clusterMigrations || []}
            esxiMigrations={esxiMigrations || []}
            migrations={migrations || []}
            refetchClusterMigrations={refetchClusterMigrations}
            refetchMigrations={refetchMigrations}
          />
        ) : (
          <BMConfigForm />
        )}
      </StyledPaper>

      <ConfirmationDialog
        open={deleteDialogOpen}
        onClose={handleDeleteClose}
        title={getDeleteDialogTitle()}
        icon={<WarningIcon color="warning" />}
        message={getDeleteDialogMessage()}
        items={getDeleteDialogItems()}
        actionLabel="Delete"
        actionColor="error"
        actionVariant="outlined"
        onConfirm={handleDeleteAction}
        customErrorMessage={getCustomErrorMessage}
        errorMessage={deleteError}
        onErrorChange={setDeleteError}
      />
    </DashboardContainer>
  )
}
