export interface RdmDisk {
  apiVersion: string
  kind: string
  metadata: {
    name: string
    namespace: string
    creationTimestamp: string
    labels?: {
      [key: string]: string
    }
    annotations?: {
      [key: string]: string
    }
  }
  spec: {
    diskName: string
    diskSize: number
    displayName: string
    importToCinder?: boolean
    openstackVolumeRef?: {
      cinderBackendPool?: string
      openstackCreds?: string
      source?: {
        [key: string]: string
      }
      volumeType?: string
    }
    ownerVMs: string[]
    uuid: string
  }
  status?: {
    cinderVolumeID?: string
    conditions?: Array<{
      lastTransitionTime: string
      message: string
      observedGeneration?: number
      reason: string
      status: "True" | "False" | "Unknown"
      type: string
    }>
    phase?: "Available" | "Managing" | "Managed" | "Error"
  }
}

export interface RdmDiskList {
  apiVersion: string
  kind: string
  metadata: {
    continue: string
    resourceVersion: string
  }
  items: RdmDisk[]
}
