import {
  useQuery,
  UseQueryOptions,
  UseQueryResult,
} from "@tanstack/react-query"
import { getVersionInfo } from "src/api/version"
import { VersionInfo } from "src/api/version/model"

export const VERSION_QUERY_KEY = ["version"]

type Options = Omit<UseQueryOptions<VersionInfo>, "queryKey" | "queryFn">

export const useVersionQuery = (
  namespace = undefined,
  options: Options = {}
): UseQueryResult<VersionInfo> => {
  return useQuery<VersionInfo>({
    queryKey: [...VERSION_QUERY_KEY, namespace],
    queryFn: async () => getVersionInfo(namespace),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: 3,
    ...options,
  })
}
