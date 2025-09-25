export interface GetMigrationTemplatesList {
  apiVersion: string
  items: MigrationTemplate[]
  kind: string
  metadata: GetMigrationTemplatesMetadata
}

export interface MigrationTemplate {
  apiVersion: string
  kind: string
  metadata: MigrationTemplateMetadata
  spec: MigrationTemplateSpec
  status: MigrationTemplateStatus
}

export interface MigrationTemplateMetadata {
  annotations: Annotations
  creationTimestamp: Date
  generation: number
  name: string
  namespace: string
  resourceVersion: string
  uid: string
  labels: Labels
}

export interface Annotations {
  "kubectl.kubernetes.io/last-applied-configuration": string
}

export interface Labels {
  refresh: string
}

export interface MigrationTemplateSpec {
  destination: Destination
  networkMapping: string
  source: Source
  storageMapping: string
  targetPCDClusterName?: string
  useFlavorless?: boolean
}

export interface Destination {
  openstackRef: string
}

export interface Source {
  vmwareRef: string
}

export interface MigrationTemplateStatus {
  openstack: Openstack
  vmware: VmData[]
}

export interface Openstack {
  networks: string[]
  volumeTypes: string[]
}

export interface VmData {
  datastores: string[]
  id: string
  name: string
  networks?: string[]
  isMigrated?: boolean
  vmState?: string
  memory?: number
  cpuCount?: number
  esxHost?: string
  ipAddress?: string
  targetFlavorId?: string
  disks?: string[]
  labels?: { [key: string]: string }
  vmWareMachineName?: string
  networkInterfaces?: VmNetworkInterface[]
  osFamily?: string
  // RDM-related properties
  rdmDisks?: string[]
  hasSharedRdm?: boolean
  rdmDependencies?: string[] // List of other VM names that share RDM disks with this VM
}

export interface VmNetworkInterface {
  mac: string
  network: string
  ipAddress: string
}

export interface GetMigrationTemplatesMetadata {
  continue: string
  resourceVersion: string
}
