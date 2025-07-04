package utils

import "errors"

var (
	ErrRDMDiskNotMigrated = errors.New("RDM disk has not been migrated yet, preventing the completion of VM migration")
)
