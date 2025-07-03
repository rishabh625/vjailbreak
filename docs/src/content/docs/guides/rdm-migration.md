---
title: RDM Migration Guide
description: Complete guide for migrating VMware Raw Device Mapping (RDM) disks to OpenStack using vJailbreak
---

# RDM Migration Guide

This guide provides comprehensive instructions for migrating VMware Raw Device Mapping (RDM) disks to OpenStack using vJailbreak. RDM migration enables you to preserve direct storage access patterns while transitioning to OpenStack infrastructure.

## Overview

Raw Device Mapping (RDM) allows VMware virtual machines to directly access physical storage devices or LUNs. vJailbreak's RDM migration capability provides seamless migration of these specialized storage configurations to OpenStack Cinder volumes while maintaining data integrity and performance characteristics.

### Key Features

- **Individual RDM Migration**: Migrate standalone RDM disks attached to single VMs
- **Shared RDM Migration**: Handle shared RDM disks used by multiple VMs (clustering scenarios)
- **Data Integrity**: Ensure complete data preservation during migration
- **Multi-attach Support**: Leverage OpenStack multi-attach volumes for shared RDM scenarios
- **Progress Monitoring**: Real-time migration progress and status tracking

### Use Cases

- **Database Clusters**: Migrate shared storage used by database clustering solutions
- **High-Performance Applications**: Preserve direct storage access for performance-critical workloads
- **Legacy Applications**: Maintain storage configurations required by legacy systems
- **Clustered Services**: Migrate multi-node services that share storage resources

## RDM Migration Concepts

### Individual vs Shared RDM

**Individual RDM**
- Single VM has exclusive access to the RDM disk
- Migrated to standard Cinder volumes
- Simpler migration process with fewer dependencies

**Shared RDM**
- Multiple VMs share access to the same physical LUN
- Requires OpenStack multi-attach capable volumes
- Coordinated migration across all sharing VMs
- Managed through SharedRDM custom resources

### Migration Strategies

| Strategy | Description | Use Case |
|----------|-------------|----------|
| `copy` | Copy RDM data to new Cinder volume | Default strategy for most scenarios |
| `reuse` | Reuse existing Cinder volume if available | When volume already exists in OpenStack |
| `manual` | Manual intervention required | Complex or unsupported configurations |

## Prerequisites

### OpenStack Requirements

#### Cinder Multi-attach Configuration

For shared RDM migration, your OpenStack environment must support multi-attach volumes:

```yaml
# cinder.conf
[DEFAULT]
enable_force_upload = True

[lvm]
volume_clear = zero
volume_clear_size = 0

# Enable multi-attach support
[oslo_concurrency]
lock_path = /var/lib/cinder/tmp
```

#### Supported Volume Types

Ensure your Cinder backend supports the required volume types:

```bash
# Check available volume types
openstack volume type list

# Verify multi-attach support
openstack volume type show <volume-type> -f value -c extra_specs
```

Required volume type properties for shared RDM:
- `multiattach: <is> True`
- Backend must support concurrent access

#### Storage Backend Requirements

- **Block Storage**: iSCSI, Fibre Channel, or NVMe backends
- **Multi-attach Support**: Backend must support concurrent volume access
- **Performance**: Adequate IOPS and bandwidth for RDM workloads

### VMware Environment

- **vCenter Access**: Administrative access to source vCenter
- **RDM Discovery**: Ability to enumerate RDM configurations
- **Network Connectivity**: Helper pods must reach both VMware and OpenStack APIs

### vJailbreak Configuration

Ensure vJailbreak is properly configured with:
- VMware credentials with sufficient privileges
- OpenStack credentials with Cinder access
- Storage mappings that include RDM-specific configurations

## Configuration

### Storage Mapping Configuration

Configure storage mappings to handle RDM disks appropriately:

```yaml
apiVersion: vjailbreak.k8s.pf9.io/v1alpha1
kind: StorageMapping
metadata:
  name: rdm-storage-mapping
  namespace: vjailbreak-system
spec:
  vmwareDatastore: "shared-storage"
  openstack:
    availabilityZone: "nova"
    volumeType: "high-performance"
    # RDM-specific configuration
    rdmVolumeType: "multi-attach-ssd"
    rdmBackendPool: "shared-pool"
    supportsMultiAttach: true
    rdmMigrationStrategy: "copy"
  # RDM-specific mappings
  rdmMappings:
    - sourceRDMType: "physical"
      targetVolumeType: "multi-attach-ssd"
      migrationMethod: "copy"
      requiresMultiAttach: true
    - sourceRDMType: "virtual"
      targetVolumeType: "standard-ssd"
      migrationMethod: "copy"
      requiresMultiAttach: false
```

### SharedRDM Resource Management

For shared RDM scenarios, create SharedRDM resources before migration:

```yaml
apiVersion: vjailbreak.k8s.pf9.io/v1alpha1
kind: SharedRDM
metadata:
  name: database-shared-storage
  namespace: vjailbreak-system
spec:
  sourceUUID: "6000c29f-4cb8-4b9d-8c7e-1234567890ab"
  displayName: "Database Cluster Shared Storage"
  diskSize: 107374182400  # 100GB in bytes
  vmwareRefs:
    - "db-node-1"
    - "db-node-2"
    - "db-node-3"
  migrationStrategy: "copy"
  cinderBackendPool: "shared-ssd-pool"
  volumeType: "multi-attach-ssd"
```

Apply the SharedRDM resource:

```bash
kubectl apply -f shared-rdm.yaml
```

Monitor SharedRDM status:

```bash
kubectl get sharedrdm database-shared-storage -o yaml
```

## Usage

### Planning RDM Migrations

1. **Discover RDM Disks**
   
   Run VM discovery to identify RDM configurations:
   
   ```bash
   kubectl get vmwaremachine -o jsonpath='{.items[*].spec.vmInfo.rdmDisks[*]}'
   ```

2. **Identify Shared RDM Scenarios**
   
   Check for shared RDM disks across multiple VMs:
   
   ```bash
   # Look for RDM disks with the same sourceUUID
   kubectl get vmwaremachine -o yaml | grep -A 5 -B 5 "rdmDisks"
   ```

3. **Create SharedRDM Resources**
   
   For shared RDM scenarios, create SharedRDM resources before starting migration:
   
   ```bash
   # Create SharedRDM resource
   kubectl apply -f shared-rdm-config.yaml
   
   # Wait for SharedRDM to be ready
   kubectl wait --for=condition=Ready sharedrdm/database-shared-storage --timeout=300s
   ```

### Executing RDM Migrations

1. **Enable RDM Migration**
   
   Configure the migration to include RDM disks:
   
   ```yaml
   apiVersion: vjailbreak.k8s.pf9.io/v1alpha1
   kind: Migration
   metadata:
     name: vm-with-rdm-migration
   spec:
     vmwareMachineRef: "database-vm-1"
     rdmMigrationEnabled: true
     rdmMigrationStrategy: "copy"
     sharedRDMRefs:
       - "database-shared-storage"
   ```

2. **Start Migration**
   
   ```bash
   kubectl apply -f migration-with-rdm.yaml
   ```

3. **Monitor Progress**
   
   Track RDM migration progress:
   
   ```bash
   # Check overall migration status
   kubectl get migration vm-with-rdm-migration -o yaml
   
   # Monitor RDM-specific progress
   kubectl get migration vm-with-rdm-migration -o jsonpath='{.status.rdmMigrationPhase}'
   ```

### Migration Phases

RDM migrations include additional phases:

1. **RDMPreparing**: Preparing RDM volumes and shared resources
2. **RDMCopying**: Copying RDM data to OpenStack volumes
3. **RDMValidating**: Validating RDM data integrity
4. **Standard Phases**: Continue with regular migration phases

## Monitoring and Status

### SharedRDM Status

Monitor SharedRDM resource status:

```bash
# Get SharedRDM status
kubectl get sharedrdm -o wide

# Detailed status information
kubectl describe sharedrdm database-shared-storage
```

SharedRDM phases:
- **Discovering**: Validating source RDM configuration
- **Creating**: Creating OpenStack volume
- **Ready**: Volume created and ready for use
- **Failed**: Error occurred during setup

### Migration Status

Track RDM migration progress:

```bash
# Check RDM migration status
kubectl get migration vm-with-rdm-migration -o jsonpath='{.status.rdmDisksCompleted}/{.status.rdmDisksTotal}'

# View RDM migration events
kubectl get events --field-selector involvedObject.name=vm-with-rdm-migration
```

### Progress Indicators

Monitor data copy progress:

```bash
# Check migration pod logs for RDM copy progress
kubectl logs -f migration-vm-with-rdm-migration-pod | grep -i rdm
```

## Troubleshooting

### Common Issues

#### SharedRDM Creation Fails

**Symptoms**: SharedRDM stuck in "Creating" phase

**Causes**:
- Insufficient OpenStack quotas
- Unsupported volume type
- Backend pool not available

**Solutions**:
```bash
# Check OpenStack quotas
openstack quota show

# Verify volume type supports multi-attach
openstack volume type show multi-attach-ssd -c extra_specs

# Check Cinder backend availability
openstack volume service list
```

#### RDM Data Copy Fails

**Symptoms**: Migration stuck in "RDMCopying" phase

**Causes**:
- Network connectivity issues
- Insufficient storage space
- Device access permissions

**Solutions**:
```bash
# Check migration pod logs
kubectl logs migration-pod-name | grep -i error

# Verify storage connectivity
kubectl exec migration-pod-name -- lsblk

# Check available space
kubectl exec migration-pod-name -- df -h
```

#### Multi-attach Volume Issues

**Symptoms**: Volume attachment fails for shared RDM

**Causes**:
- Backend doesn't support multi-attach
- Volume type misconfiguration
- Instance limits exceeded

**Solutions**:
```bash
# Verify multi-attach capability
openstack volume show <volume-id> -c multiattach

# Check instance attachment limits
openstack server show <instance-id> -c volumes_attached
```

### Diagnostic Commands

```bash
# Check RDM discovery results
kubectl get vmwaremachine -o jsonpath='{.items[*].spec.vmInfo.rdmDisks}'

# Verify SharedRDM resources
kubectl get sharedrdm -A

# Check migration controller logs
kubectl logs -n vjailbreak-system deployment/migration-controller | grep -i rdm

# Examine migration pod events
kubectl describe pod migration-pod-name
```

### Log Analysis

Key log patterns to look for:

```bash
# RDM discovery logs
grep "RDM disk discovered" /var/log/vjailbreak/discovery.log

# Data copy progress
grep "RDM copy progress" /var/log/vjailbreak/migration.log

# Volume creation logs
grep "Creating multi-attach volume" /var/log/vjailbreak/openstack.log
```

## Limitations

### Current Limitations

- **Physical RDM Only**: Virtual RDM support is limited
- **Single Backend**: All RDM volumes must use the same Cinder backend
- **No Live Migration**: RDM data copy requires VM downtime
- **Size Limits**: Maximum RDM disk size depends on Cinder backend limits

### Unsupported Scenarios

- **Raw Device Mapping to Files**: File-backed RDM not supported
- **Cross-Backend Shared RDM**: Shared RDM across different storage backends
- **Dynamic RDM Expansion**: Online RDM disk expansion during migration
- **Snapshot-based Migration**: RDM migration via snapshots not supported

### OpenStack Version Requirements

- **Minimum Version**: OpenStack Train (for stable multi-attach)
- **Recommended**: OpenStack Ussuri or later
- **Cinder API**: Version 3.27 or later for multi-attach support

## Best Practices

### Pre-migration Planning

1. **Inventory RDM Usage**
   ```bash
   # Document all RDM configurations
   kubectl get vmwaremachine -o yaml > rdm-inventory.yaml
   ```

2. **Validate OpenStack Capacity**
   ```bash
   # Check available storage quota
   openstack quota show --volume
   ```

3. **Test Multi-attach Support**
   ```bash
   # Create test multi-attach volume
   openstack volume create --size 1 --type multi-attach-ssd test-volume
   ```

### Migration Execution

1. **Staged Migration**: Migrate non-shared RDM disks first
2. **Coordinate Shared RDM**: Ensure all VMs using shared RDM are migrated together
3. **Monitor Progress**: Use real-time monitoring during data copy phases
4. **Validate Data**: Perform integrity checks after migration

### Post-migration Validation

1. **Verify Volume Attachments**
   ```bash
   openstack server show <instance-id> -c volumes_attached
   ```

2. **Test Application Functionality**
   - Verify database cluster operations
   - Test application data access
   - Validate performance characteristics

3. **Monitor Performance**
   - Compare IOPS and latency metrics
   - Monitor storage backend utilization
   - Validate multi-attach behavior

### Performance Optimization

1. **Backend Selection**: Use high-performance backends for RDM volumes
2. **Network Optimization**: Ensure adequate bandwidth for data copy
3. **Parallel Migration**: Migrate multiple non-shared RDM disks in parallel
4. **Resource Allocation**: Allocate sufficient CPU and memory for migration pods

## Related Documentation

- [Storage Mapping Configuration](../storage-mappings/)
- [Migration Planning Guide](../migration-planning/)
- [OpenStack Integration](../openstack-setup/)
- [Troubleshooting Guide](../troubleshooting/)

## External Resources

- [OpenStack Cinder Multi-attach Documentation](https://docs.openstack.org/cinder/latest/admin/blockstorage-volume-multiattach.html)
- [VMware RDM Documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.storage.doc/GUID-EEED2222-A5F0-4CD5-AE96-7C25CCF0E2A8.html)
- [Kubernetes Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)