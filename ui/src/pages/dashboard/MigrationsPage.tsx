import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { FIVE_SECONDS, THIRTY_SECONDS } from "src/constants"
import { useMigrationsQuery, MIGRATIONS_QUERY_KEY } from "src/hooks/api/useMigrationsQuery"
import { deleteMigration } from "src/api/migrations/migrations"
import { getMigrationPlan, patchMigrationPlan } from "src/api/migration-plans/migrationPlans"
import ConfirmationDialog from "src/components/dialogs/ConfirmationDialog"
import { Migration } from "src/api/migrations/model"
import MigrationsTable from "./MigrationsTable"
import WarningIcon from '@mui/icons-material/Warning'

export default function MigrationsPage() {
  const queryClient = useQueryClient()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [selectedMigrations, setSelectedMigrations] = useState<Migration[]>([])
  const [deleteError, setDeleteError] = useState<string | null>(null)

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

  const handleDeleteClick = (migrationName: string) => {
    const migration = migrations?.find(m => m.metadata.name === migrationName)
    if (migration) {
      setSelectedMigrations([migration])
      setDeleteDialogOpen(true)
    }
  }

  const handleDeleteClose = () => {
    setDeleteDialogOpen(false)
    setSelectedMigrations([])
    setDeleteError(null)
  }

  const handleDeleteSelected = (migrations: Migration[]) => {
    setSelectedMigrations(migrations)
    setDeleteDialogOpen(true)
  }

  const handleDeleteMigration = async (migrations: Migration[]) => {
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

  const getCustomErrorMessage = (error: Error | string) => {
    const baseMessage = "Failed to delete migrations"
    if (error instanceof Error) {
      return `${baseMessage}: ${error.message}`
    }
    return baseMessage
  }

  return (
    <>
      <MigrationsTable
        refetchMigrations={refetchMigrations}
        migrations={migrations || []}
        onDeleteMigration={handleDeleteClick}
        onDeleteSelected={handleDeleteSelected}
      />

      <ConfirmationDialog
        open={deleteDialogOpen}
        onClose={handleDeleteClose}
        title="Confirm Delete Migration"
        icon={<WarningIcon color="warning" />}
        message={
          selectedMigrations.length > 1
            ? "Are you sure you want to delete these migrations?"
            : `Are you sure you want to delete migration "${selectedMigrations[0]?.metadata.name}"?`
        }
        items={selectedMigrations.map(m => ({
          id: m.metadata.name,
          name: m.metadata.name
        }))}
        actionLabel="Delete"
        actionColor="error"
        actionVariant="outlined"
        onConfirm={() => handleDeleteMigration(selectedMigrations)}
        customErrorMessage={getCustomErrorMessage}
        errorMessage={deleteError}
        onErrorChange={setDeleteError}
      />
    </>
  )
}