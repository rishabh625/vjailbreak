import RollingMigrationsTable from "./RollingMigrationsTable"
import { useMigrationsQuery } from "src/hooks/api/useMigrationsQuery"
import { useClusterMigrationsQuery, CLUSTER_MIGRATIONS_QUERY_KEY } from "src/hooks/api/useClusterMigrationsQuery"
import { useESXIMigrationsQuery, ESXI_MIGRATIONS_QUERY_KEY } from "src/hooks/api/useESXIMigrationsQuery"
import { THIRTY_SECONDS } from "src/constants"

export default function ClusterConversionsPage() {
  const { data: migrations, refetch: refetchMigrations } = useMigrationsQuery()
  
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

  return (
    <RollingMigrationsTable
      clusterMigrations={clusterMigrations || []}
      esxiMigrations={esxiMigrations || []}
      migrations={migrations || []}
      refetchClusterMigrations={refetchClusterMigrations}
      refetchMigrations={refetchMigrations}
    />
  )
}