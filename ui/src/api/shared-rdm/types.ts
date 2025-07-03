/**
 * TypeScript type definitions for SharedRDM resources
 * These types match the Kubernetes API structure for SharedRDM custom resources
 */

/**
 * Kubernetes metadata interface for SharedRDM resources
 */
export interface SharedRDMMetadata {
  /** Unique name for the SharedRDM resource within a namespace */
  name: string;
  /** Namespace where the SharedRDM resource is created */
  namespace?: string;
  /** Unique identifier assigned by Kubernetes */
  uid?: string;
  /** Resource version for optimistic concurrency control */
  resourceVersion?: string;
  /** Creation timestamp */
  creationTimestamp?: string;
  /** Deletion timestamp if resource is being deleted */
  deletionTimestamp?: string;
  /** Labels for organizing and selecting SharedRDM resources */
  labels?: Record<string, string>;
  /** Annotations for additional metadata */
  annotations?: Record<string, string>;
  /** Finalizers for cleanup coordination */
  finalizers?: string[];
}

/**
 * OpenStack volume reference information
 */
export interface OpenStackVolumeRefInfo {
  /** OpenStack volume ID */
  volumeID: string;
  /** Volume name in OpenStack */
  volumeName: string;
  /** OpenStack project/tenant ID */
  projectID: string;
  /** Volume size in bytes */
  size: number;
  /** Volume type used in OpenStack */
  volumeType: string;
  /** Backend pool where volume is created */
  backendPool?: string;
  /** Whether multi-attach is enabled */
  multiAttachEnabled: boolean;
}

/**
 * Migration strategy enumeration for SharedRDM
 */
export enum SharedRDMMigrationStrategy {
  /** Copy data from VMware RDM to new OpenStack volume */
  COPY = 'copy',
  /** Reuse existing OpenStack volume if available */
  REUSE = 'reuse',
  /** Manual migration - user handles data migration */
  MANUAL = 'manual',
  /** Skip migration of this RDM disk */
  SKIP = 'skip'
}

/**
 * SharedRDM phase enumeration
 */
export enum SharedRDMPhase {
  /** Initial phase - discovering RDM information */
  DISCOVERING = 'Discovering',
  /** Creating OpenStack volume */
  CREATING = 'Creating',
  /** Volume created and ready for use */
  READY = 'Ready',
  /** Migration in progress */
  MIGRATING = 'Migrating',
  /** Migration completed successfully */
  COMPLETED = 'Completed',
  /** Error occurred during processing */
  FAILED = 'Failed',
  /** Resource is being deleted */
  DELETING = 'Deleting'
}

/**
 * Condition type enumeration for SharedRDM status
 */
export enum SharedRDMConditionType {
  /** Volume is ready for use */
  VOLUME_READY = 'VolumeReady',
  /** Data migration is complete */
  MIGRATION_COMPLETE = 'MigrationComplete',
  /** Multi-attach is configured */
  MULTI_ATTACH_ENABLED = 'MultiAttachEnabled',
  /** Error condition */
  ERROR = 'Error'
}

/**
 * Condition status enumeration
 */
export enum ConditionStatus {
  TRUE = 'True',
  FALSE = 'False',
  UNKNOWN = 'Unknown'
}

/**
 * Status condition for SharedRDM resources
 */
export interface SharedRDMCondition {
  /** Type of condition */
  type: SharedRDMConditionType;
  /** Status of the condition */
  status: ConditionStatus;
  /** Last time the condition transitioned */
  lastTransitionTime: string;
  /** Machine-readable reason for the condition */
  reason: string;
  /** Human-readable message describing the condition */
  message: string;
  /** Last time the condition was probed */
  lastProbeTime?: string;
}

/**
 * SharedRDM specification defining the desired state
 */
export interface SharedRDMSpec {
  /** The VMware LUN UUID that identifies the source RDM disk */
  sourceUUID: string;
  
  /** Human-readable name for the shared RDM */
  displayName: string;
  
  /** Size of the RDM disk in bytes */
  diskSize: number;
  
  /** List of VMware VM names that use this shared RDM */
  vmwareRefs: string[];
  
  /** Reference to the created OpenStack volume */
  openstackVolumeRef?: OpenStackVolumeRefInfo;
  
  /** Strategy for migrating the RDM data */
  migrationStrategy: SharedRDMMigrationStrategy;
  
  /** Target Cinder backend pool for the volume */
  cinderBackendPool?: string;
  
  /** Target OpenStack volume type */
  volumeType: string;
  
  /** Whether to enable multi-attach for shared scenarios */
  enableMultiAttach?: boolean;
  
  /** Custom metadata for the SharedRDM */
  metadata?: Record<string, string>;
}

/**
 * SharedRDM status representing the current state
 */
export interface SharedRDMStatus {
  /** Current phase of the SharedRDM resource */
  phase: SharedRDMPhase;
  
  /** Status conditions providing detailed state information */
  conditions: SharedRDMCondition[];
  
  /** Created OpenStack volume ID */
  volumeID?: string;
  
  /** List of VM names currently using this volume */
  attachedVMs: string[];
  
  /** Migration progress percentage (0-100) */
  migrationProgress?: number;
  
  /** Last error message if any */
  lastError?: string;
  
  /** Timestamp of last status update */
  lastUpdated?: string;
  
  /** Number of bytes migrated */
  bytesMigrated?: number;
  
  /** Total bytes to migrate */
  totalBytes?: number;
  
  /** Migration start time */
  migrationStartTime?: string;
  
  /** Migration completion time */
  migrationCompletionTime?: string;
  
  /** Observed generation of the spec */
  observedGeneration?: number;
}

/**
 * Complete SharedRDM resource definition
 */
export interface SharedRDM {
  /** API version for the SharedRDM resource */
  apiVersion: string;
  
  /** Resource kind - always "SharedRDM" */
  kind: string;
  
  /** Kubernetes metadata */
  metadata: SharedRDMMetadata;
  
  /** Desired state specification */
  spec: SharedRDMSpec;
  
  /** Current state status */
  status?: SharedRDMStatus;
}

/**
 * List response for SharedRDM resources
 */
export interface SharedRDMList {
  /** API version for the list */
  apiVersion: string;
  
  /** Resource kind - always "SharedRDMList" */
  kind: string;
  
  /** List metadata */
  metadata: {
    /** Resource version for the list */
    resourceVersion?: string;
    /** Continue token for pagination */
    continue?: string;
    /** Remaining items count */
    remainingItemCount?: number;
  };
  
  /** Array of SharedRDM resources */
  items: SharedRDM[];
}

/**
 * Request payload for creating a SharedRDM resource
 */
export interface CreateSharedRDMRequest {
  /** SharedRDM specification */
  spec: SharedRDMSpec;
  
  /** Optional metadata */
  metadata?: Partial<SharedRDMMetadata>;
}

/**
 * Request payload for updating a SharedRDM resource
 */
export interface UpdateSharedRDMRequest {
  /** Updated SharedRDM specification */
  spec: Partial<SharedRDMSpec>;
  
  /** Optional metadata updates */
  metadata?: Partial<SharedRDMMetadata>;
}

/**
 * Filter options for listing SharedRDM resources
 */
export interface SharedRDMListOptions {
  /** Filter by namespace */
  namespace?: string;
  
  /** Label selector */
  labelSelector?: string;
  
  /** Field selector */
  fieldSelector?: string;
  
  /** Filter by phase */
  phase?: SharedRDMPhase;
  
  /** Filter by migration strategy */
  migrationStrategy?: SharedRDMMigrationStrategy;
  
  /** Limit number of results */
  limit?: number;
  
  /** Continue token for pagination */
  continue?: string;
}

/**
 * SharedRDM validation error
 */
export interface SharedRDMValidationError {
  /** Field path where error occurred */
  field: string;
  
  /** Error message */
  message: string;
  
  /** Error code */
  code: string;
}

/**
 * Validation result for SharedRDM configuration
 */
export interface SharedRDMValidationResult {
  /** Whether validation passed */
  valid: boolean;
  
  /** List of validation errors */
  errors: SharedRDMValidationError[];
  
  /** List of validation warnings */
  warnings: SharedRDMValidationError[];
}

/**
 * SharedRDM metrics for monitoring
 */
export interface SharedRDMMetrics {
  /** Total number of SharedRDM resources */
  total: number;
  
  /** Count by phase */
  byPhase: Record<SharedRDMPhase, number>;
  
  /** Count by migration strategy */
  byStrategy: Record<SharedRDMMigrationStrategy, number>;
  
  /** Average migration time in seconds */
  averageMigrationTime?: number;
  
  /** Success rate percentage */
  successRate?: number;
}

/**
 * Event related to SharedRDM operations
 */
export interface SharedRDMEvent {
  /** Event type */
  type: 'Normal' | 'Warning';
  
  /** Event reason */
  reason: string;
  
  /** Event message */
  message: string;
  
  /** Event timestamp */
  timestamp: string;
  
  /** Source component */
  source: string;
  
  /** Related SharedRDM resource */
  involvedObject: {
    name: string;
    namespace: string;
    uid: string;
  };
}

/**
 * Type guard to check if an object is a SharedRDM
 */
export function isSharedRDM(obj: any): obj is SharedRDM {
  return obj && 
         obj.apiVersion === 'vjailbreak.k8s.pf9.io/v1alpha1' && 
         obj.kind === 'SharedRDM' &&
         obj.metadata &&
         obj.spec;
}

/**
 * Type guard to check if an object is a SharedRDMList
 */
export function isSharedRDMList(obj: any): obj is SharedRDMList {
  return obj && 
         obj.apiVersion === 'vjailbreak.k8s.pf9.io/v1alpha1' && 
         obj.kind === 'SharedRDMList' &&
         Array.isArray(obj.items);
}

/**
 * Helper function to create a default SharedRDMSpec
 */
export function createDefaultSharedRDMSpec(): Partial<SharedRDMSpec> {
  return {
    migrationStrategy: SharedRDMMigrationStrategy.COPY,
    volumeType: 'standard',
    enableMultiAttach: true,
    vmwareRefs: [],
    metadata: {}
  };
}

/**
 * Helper function to get condition by type
 */
export function getConditionByType(
  conditions: SharedRDMCondition[], 
  type: SharedRDMConditionType
): SharedRDMCondition | undefined {
  return conditions.find(condition => condition.type === type);
}

/**
 * Helper function to check if SharedRDM is ready
 */
export function isSharedRDMReady(sharedRDM: SharedRDM): boolean {
  return sharedRDM.status?.phase === SharedRDMPhase.READY ||
         sharedRDM.status?.phase === SharedRDMPhase.COMPLETED;
}

/**
 * Helper function to check if SharedRDM has errors
 */
export function hasSharedRDMErrors(sharedRDM: SharedRDM): boolean {
  return sharedRDM.status?.phase === SharedRDMPhase.FAILED ||
         sharedRDM.status?.conditions?.some(c => c.type === SharedRDMConditionType.ERROR && c.status === ConditionStatus.TRUE) ||
         false;
}