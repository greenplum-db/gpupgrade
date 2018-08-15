package utils

import (
	"fmt"
	"path/filepath"
)

// PGUpgradeDirectory returns a path to a directory underneath the state
// directory that is to be used for storing pg_upgrade state. It does not ensure
// the directory exists first.
func PGUpgradeDirectory(stateDir string) string {
	return filepath.Join(stateDir, "pg_upgrade")
}

// SegmentPGUpgradeDirectory returns a path to a directory underneath the
// PGUpgradeDirectory that is to be used for pg_upgrade state for a given
// segment. Like PGUpgradeDirectory, it does not ensure that the directory
// exists.
func SegmentPGUpgradeDirectory(stateDir string, contentID int) string {
	return filepath.Join(PGUpgradeDirectory(stateDir), fmt.Sprintf("seg%d", contentID))
}

// MasterPGUpgradeDirectory is a convenience method equivalent to
// SegmentPGUpgradeDirectory(stateDir, -1).
func MasterPGUpgradeDirectory(stateDir string) string {
	return SegmentPGUpgradeDirectory(stateDir, -1)
}
