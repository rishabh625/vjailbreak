// Package verrors defines custom error types for the VM migration process.
package verrors

import "errors"

// ErrRDMDiskNotMigrated is an error that indicates that the RDM disk has not been able to be migrated yet,
var ErrRDMDiskNotMigrated = errors.New("RDM disk has not been migrated yet, preventing the completion of VM migration")
