import { get } from "../axios"
import { VersionConfigMap, VersionInfo } from "./model"

const VERSION_CONFIG_MAP_NAME = "version-config"
const VERSION_NAMESPACE = "migration-system"

export const getVersionConfigMap = async (
  namespace: string = VERSION_NAMESPACE
): Promise<VersionConfigMap> => {
  const endpoint = `/api/v1/namespaces/${namespace}/configmaps/${VERSION_CONFIG_MAP_NAME}`
  return get<VersionConfigMap>({
    endpoint,
    config: { mock: false }, // Force real API call, not mock
  })
}

export const getVersionInfo = async (
  namespace: string = VERSION_NAMESPACE
): Promise<VersionInfo> => {
  try {
    const configMap = await getVersionConfigMap(namespace)
    return {
      version: configMap.data.version,
      upgradeAvailable: configMap.data.upgradeAvailable === "true",
      upgradeVersion: configMap.data.upgradeVersion,
    }
  } catch (error) {
    console.error("Failed to fetch version info:", error)
    throw error
  }
}
