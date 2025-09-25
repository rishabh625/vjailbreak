import { useQuery } from "@tanstack/react-query"
import { getRdmDisksList } from "src/api/rdm-disks/rdmDisks"
import { RdmDisk } from "src/api/rdm-disks/model"

export const RDM_DISKS_BASE_KEY = "rdmdisks"

interface UseRdmDisksQueryProps {
  enabled?: boolean
  namespace?: string
}

export const useRdmDisksQuery = ({
  enabled = true,
  namespace,
}: UseRdmDisksQueryProps = {}) => {
  return useQuery({
    queryKey: [RDM_DISKS_BASE_KEY, namespace],
    queryFn: async (): Promise<RdmDisk[]> => {
      return getRdmDisksList(namespace)
    },
    enabled,
    refetchOnWindowFocus: false,
    staleTime: 1000 * 60 * 5, // 5 minutes
    placeholderData: [],
  })
}
