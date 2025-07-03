import { get, post, put, del } from "../axios"

// SharedRDM API types
export interface SharedRDMSpec {
  sourceUUID: string
  displayName: string
  diskSize: number
  vmwareRefs: string[]
  openstackVolumeRef?: OpenStackVolumeRefInfo
  migrationStrategy: string
  cinderBackendPool?: string
  volumeType?: string
}

export interface OpenStackVolumeRefInfo {
  volumeID: string
  volumeName: string
  volumeType: string
  size: number
}

export interface SharedRDMCondition {
  type: string
  status: string
  lastTransitionTime: string
  reason: string
  message: string
}

export interface SharedRDMStatus {
  phase: string
  conditions: SharedRDMCondition[]
  volumeID?: string
  attachedVMs: string[]
}

export interface SharedRDM {
  apiVersion: string
  kind: string
  metadata: {
    name: string
    namespace: string
    creationTimestamp?: string
    resourceVersion?: string
    uid?: string
  }
  spec: SharedRDMSpec
  status?: SharedRDMStatus
}

export interface SharedRDMList {
  apiVersion: string
  kind: string
  items: SharedRDM[]
  metadata: {
    continue?: string
    resourceVersion: string
  }
}

export interface SharedRDMListParams {
  namespace?: string
  labelSelector?: string
  fieldSelector?: string
  limit?: number
  continue?: string
}

export interface SharedRDMValidationError {
  field: string
  message: string
}

export interface SharedRDMValidationResult {
  valid: boolean
  errors: SharedRDMValidationError[]
}

// API endpoints
const SHARED_RDM_BASE_PATH = "/api/v1/namespaces"
const SHARED_RDM_RESOURCE = "sharedrdms"
const API_GROUP = "vjailbreak.k8s.pf9.io/v1alpha1"

// Helper function to build resource path
const buildResourcePath = (namespace: string = "default", name?: string) => {
  const basePath = `${SHARED_RDM_BASE_PATH}/${namespace}/${SHARED_RDM_RESOURCE}`
  return name ? `${basePath}/${name}` : basePath
}

// Helper function to build query string
const buildQueryString = (params: SharedRDMListParams = {}) => {
  const queryParams = new URLSearchParams()
  
  if (params.labelSelector) queryParams.append("labelSelector", params.labelSelector)
  if (params.fieldSelector) queryParams.append("fieldSelector", params.fieldSelector)
  if (params.limit) queryParams.append("limit", params.limit.toString())
  if (params.continue) queryParams.append("continue", params.continue)
  
  const queryString = queryParams.toString()
  return queryString ? `?${queryString}` : ""
}

/**
 * Get list of SharedRDM resources
 */
export const getSharedRDMs = async (
  namespace: string = "default",
  params: SharedRDMListParams = {}
): Promise<SharedRDMList> => {
  const endpoint = `${buildResourcePath(namespace)}${buildQueryString(params)}`
  
  return get<SharedRDMList>({
    endpoint,
    config: { mock: false }
  })
}

/**
 * Get specific SharedRDM resource
 */
export const getSharedRDM = async (
  name: string,
  namespace: string = "default"
): Promise<SharedRDM> => {
  const endpoint = buildResourcePath(namespace, name)
  
  return get<SharedRDM>({
    endpoint,
    config: { mock: false }
  })
}

/**
 * Create new SharedRDM resource
 */
export const createSharedRDM = async (
  spec: SharedRDMSpec,
  name: string,
  namespace: string = "default"
): Promise<SharedRDM> => {
  const endpoint = buildResourcePath(namespace)
  
  const sharedRDM: Partial<SharedRDM> = {
    apiVersion: API_GROUP,
    kind: "SharedRDM",
    metadata: {
      name,
      namespace
    },
    spec
  }
  
  return post<SharedRDM>({
    endpoint,
    data: sharedRDM,
    config: { mock: false }
  })
}

/**
 * Update SharedRDM resource
 */
export const updateSharedRDM = async (
  name: string,
  spec: SharedRDMSpec,
  namespace: string = "default"
): Promise<SharedRDM> => {
  const endpoint = buildResourcePath(namespace, name)
  
  // Get current resource to preserve metadata
  const currentResource = await getSharedRDM(name, namespace)
  
  const updatedResource: SharedRDM = {
    ...currentResource,
    spec
  }
  
  return put<SharedRDM>({
    endpoint,
    data: updatedResource,
    config: { mock: false }
  })
}

/**
 * Delete SharedRDM resource
 */
export const deleteSharedRDM = async (
  name: string,
  namespace: string = "default"
): Promise<void> => {
  const endpoint = buildResourcePath(namespace, name)
  
  return del<void>({
    endpoint,
    config: { mock: false }
  })
}

/**
 * Get SharedRDM status
 */
export const getSharedRDMStatus = async (
  name: string,
  namespace: string = "default"
): Promise<SharedRDMStatus | undefined> => {
  const sharedRDM = await getSharedRDM(name, namespace)
  return sharedRDM.status
}

/**
 * Validate SharedRDM configuration
 */
export const validateSharedRDMConfig = (spec: SharedRDMSpec): SharedRDMValidationResult => {
  const errors: SharedRDMValidationError[] = []
  
  // Required field validations
  if (!spec.sourceUUID || spec.sourceUUID.trim() === "") {
    errors.push({
      field: "sourceUUID",
      message: "Source UUID is required"
    })
  }
  
  if (!spec.displayName || spec.displayName.trim() === "") {
    errors.push({
      field: "displayName",
      message: "Display name is required"
    })
  }
  
  if (!spec.diskSize || spec.diskSize <= 0) {
    errors.push({
      field: "diskSize",
      message: "Disk size must be greater than 0"
    })
  }
  
  if (!spec.vmwareRefs || spec.vmwareRefs.length === 0) {
    errors.push({
      field: "vmwareRefs",
      message: "At least one VMware VM reference is required"
    })
  }
  
  if (!spec.migrationStrategy || spec.migrationStrategy.trim() === "") {
    errors.push({
      field: "migrationStrategy",
      message: "Migration strategy is required"
    })
  }
  
  // Format validations
  if (spec.sourceUUID && !/^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(spec.sourceUUID)) {
    errors.push({
      field: "sourceUUID",
      message: "Source UUID must be a valid UUID format"
    })
  }
  
  // Migration strategy validation
  const validStrategies = ["copy", "reuse", "manual", "skip"]
  if (spec.migrationStrategy && !validStrategies.includes(spec.migrationStrategy)) {
    errors.push({
      field: "migrationStrategy",
      message: `Migration strategy must be one of: ${validStrategies.join(", ")}`
    })
  }
  
  // VMware refs validation
  if (spec.vmwareRefs) {
    spec.vmwareRefs.forEach((ref, index) => {
      if (!ref || ref.trim() === "") {
        errors.push({
          field: `vmwareRefs[${index}]`,
          message: "VMware VM reference cannot be empty"
        })
      }
    })
  }
  
  // Disk size validation (minimum 1GB)
  if (spec.diskSize && spec.diskSize < 1073741824) {
    errors.push({
      field: "diskSize",
      message: "Disk size must be at least 1GB (1073741824 bytes)"
    })
  }
  
  return {
    valid: errors.length === 0,
    errors
  }
}

/**
 * Get SharedRDMs by VMware VM reference
 */
export const getSharedRDMsByVM = async (
  vmName: string,
  namespace: string = "default"
): Promise<SharedRDM[]> => {
  const allSharedRDMs = await getSharedRDMs(namespace)
  
  return allSharedRDMs.items.filter(sharedRDM =>
    sharedRDM.spec.vmwareRefs.includes(vmName)
  )
}

/**
 * Get SharedRDMs by status phase
 */
export const getSharedRDMsByPhase = async (
  phase: string,
  namespace: string = "default"
): Promise<SharedRDM[]> => {
  const allSharedRDMs = await getSharedRDMs(namespace)
  
  return allSharedRDMs.items.filter(sharedRDM =>
    sharedRDM.status?.phase === phase
  )
}

/**
 * Check if SharedRDM is ready
 */
export const isSharedRDMReady = (sharedRDM: SharedRDM): boolean => {
  return sharedRDM.status?.phase === "Ready"
}

/**
 * Get SharedRDM conditions by type
 */
export const getSharedRDMCondition = (
  sharedRDM: SharedRDM,
  conditionType: string
): SharedRDMCondition | undefined => {
  return sharedRDM.status?.conditions.find(condition => condition.type === conditionType)
}

/**
 * Watch SharedRDM status changes (polling-based)
 */
export const watchSharedRDMStatus = (
  name: string,
  namespace: string = "default",
  callback: (status: SharedRDMStatus | undefined) => void,
  intervalMs: number = 5000
): () => void => {
  let isWatching = true
  
  const poll = async () => {
    if (!isWatching) return
    
    try {
      const status = await getSharedRDMStatus(name, namespace)
      callback(status)
    } catch (error) {
      console.error(`Error watching SharedRDM ${name}:`, error)
    }
    
    if (isWatching) {
      setTimeout(poll, intervalMs)
    }
  }
  
  // Start polling
  poll()
  
  // Return cleanup function
  return () => {
    isWatching = false
  }
}

export default {
  getSharedRDMs,
  getSharedRDM,
  createSharedRDM,
  updateSharedRDM,
  deleteSharedRDM,
  getSharedRDMStatus,
  validateSharedRDMConfig,
  getSharedRDMsByVM,
  getSharedRDMsByPhase,
  isSharedRDMReady,
  getSharedRDMCondition,
  watchSharedRDMStatus
}