export interface GetOpenstackCredsList {
  apiVersion: string
  items: OpenstackCreds[]
  kind: string
  metadata: GetOpenstackCredsListMetadata
}

export interface OpenstackCreds {
  apiVersion: string
  kind: string
  metadata: OpenstackCredsMetadata
  spec: OpenstackCredsSpec
  status: OpenstackCredsStatus
}

export interface OpenstackCredsMetadata {
  annotations: Annotations
  creationTimestamp: Date
  generation: number
  name: string
  namespace: string
  resourceVersion: string
  uid: string
}

export interface Annotations {
  "kubectl.kubernetes.io/last-applied-configuration": string
}

export interface OpenstackFlavor {
  id: string
  name: string
  vcpus: number
  ram: number
  disk: number
}

export interface PCDHostConfig {
  clusterName: string
  hostLivenessInterface: string
  id: string
  imagelibInterface: string
  mgmtInterface: string
  name: string
  networkLabels: Record<string, string>
  tunnelingInterface: string
  vmConsoleInterface: string
}

export interface OpenstackCredsSpec {
  // For credentials using secretRef (new approach)
  secretRef?: {
    name: string
  }
  flavors?: OpenStackFlavor[]
  pcdHostConfig?: PCDHostConfig[]
}

export interface GetOpenstackCredsListMetadata {
  continue: string
  resourceVersion: string
}

export interface OpenstackCredsStatus {
  openstackValidationMessage: string
  openstackValidationStatus: string
  openstack?: {
    networks?: string[]
    volumeTypes?: string[]
  }
}

export interface OpenstackImage {
  name: string
  id: string
  status: string
  virtual_size: number
}

export interface OpenstackImagesResponse {
  images: OpenstackImage[]
}

export interface OpenStackFlavor {
  "OS-FLV-EXT-DATA:ephemeral"?: number
  description?: string
  disk: number
  id: string
  name: string
  "os-flavor-access:is_public"?: boolean
  ram: number
  rxtx_factor?: number
  vcpus: number
}
