export interface VersionConfigMap {
  apiVersion: string
  kind: string
  metadata: {
    name: string
    namespace: string
    annotations?: Record<string, string>
  }
  data: {
    upgradeAvailable: string
    upgradeVersion: string
    version: string
  }
}

export interface VersionInfo {
  version: string
  upgradeAvailable: boolean
  upgradeVersion: string
}
