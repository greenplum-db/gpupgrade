// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"
const OldSuffix = ".old"

var PostgresFiles = []string{"postgresql.conf", "PG_VERSION"}
var StateDirectoryFiles = []string{"config.json", "status.json"}

func GetConfigFile() string {
	return filepath.Join(utils.GetStateDir(), ConfigFileName)
}

// pgUpgradeDirectory returns a path to a directory underneath the state
// directory that is to be used for storing pg_upgrade state. It does not ensure
// the directory exists first.
func pgUpgradeDirectory(stateDir string) string {
	return filepath.Join(stateDir, "pg_upgrade")
}

// SegmentWorkingDirectory returns a path to a directory underneath the
// pgUpgradeDirectory that is to be used for pg_upgrade state for a given
// segment. Like pgUpgradeDirectory, it does not ensure that the directory
// exists.
func SegmentWorkingDirectory(stateDir string, contentID int) string {
	return filepath.Join(pgUpgradeDirectory(stateDir), fmt.Sprintf("seg%d", contentID))
}

// MasterWorkingDirectory is a convenience method equivalent to
// SegmentWorkingDirectory(stateDir, -1).
func MasterWorkingDirectory(stateDir string) string {
	return SegmentWorkingDirectory(stateDir, -1)
}

// TempDataDir transforms a data directory into a corresponding temporary path
// suitable for an upgrade target, using the desired cluster segment prefix and
// upgrade ID for uniqification.
//
// The rules are currently as follows due to 6X gpinitsystem requirements:
//
// - The temporary datadir will be placed next to the original datadir.
//
// - If the datadir basename starts with the segment prefix, the remainder of
// the basename is considered the segment suffix. The temporary datadir will
// also start with the segment prefix and end with the segment suffix.
//
// - If the datadir basename does not start with the segment prefix (as can
// happen with e.g. standby data directories), the temporary datadir will
// start with the original basename.
func TempDataDir(datadir, segPrefix string, id ID) string {
	datadir = filepath.Clean(datadir) // sanitize trailing slashes for Split
	dir, base := filepath.Split(datadir)

	var newBase string
	if strings.HasPrefix(base, segPrefix) {
		suffix := strings.TrimPrefix(base, segPrefix)
		newBase = fmt.Sprintf("%s.%s.%s", segPrefix, id, suffix)
	} else {
		newBase = fmt.Sprintf("%s.%s", base, id)
	}

	return filepath.Join(dir, newBase)
}

// GetArchiveDirectoryName returns the name of the file to be used to store logs
//   from this run of gpupgrade during a revert.
func GetArchiveDirectoryName(id ID, t time.Time) string {
	return fmt.Sprintf("gpupgrade-%s-%s", id.String(), t.Format("2006-01-02T15:04"))
}

// ArchiveSource archives the source directory, and renames
// source to target. For example:
//   source '/data/dbfast1/demoDataDir0' becomes archive '/data/dbfast1/demoDataDir.123ABC.0.old'
//   target '/data/dbfast1/demoDataDir.123ABC.0' becomes source '/data/dbfast1/demoDataDir0'
// When renameTarget is false just the source directory is archived. This is
// useful in link mode when the mirrors have been deleted to save disk space and
// will upgraded later to their correct location. Thus, renameTarget is false in
// link mode when there is only the source directory to archive.
func ArchiveSource(source, target string, renameTarget bool) error {
	// Instead of manipulating the source to create the archive we append the
	// old suffix to the target to achieve the same result.
	archive := target + OldSuffix
	if alreadyRenamed(archive, target) {
		return nil
	}

	if PathExists(source) {
		if err := renameDataDirectory(source, archive); err != nil {
			return err
		}
	} else {
		gplog.Debug("Source directory not found when renaming %q to %q. It was already renamed from a previous run.", source, archive)
	}

	// In link mode mirrors have been deleted to save disk space, so there is
	// no target to rename. Only archiving the source is needed.
	if !renameTarget {
		return nil
	}

	if err := renameDataDirectory(target, source); err != nil {
		return err
	}

	return nil
}

func renameDataDirectory(src, dst string) error {
	if err := VerifyDataDirectory(src); err != nil {
		return err
	}

	if err := utils.System.Rename(src, dst); err != nil {
		return err
	}

	return nil
}

// ErrInvalidDataDirectory is returned when a data directory does not look like
// a postgres data directory, and is returned by ArchiveAndSwapDirectories.
var ErrInvalidDataDirectory = errors.New("invalid data directory")

// InvalidDataDirectoryError is the backing error type for
// ErrInvalidDataDirectory.
type InvalidDataDirectoryError struct {
	path string
	file string
}

func (i *InvalidDataDirectoryError) Error() string {
	return fmt.Sprintf("%q does not look like a postgres directory. Failed to find %q", i.path, i.file)
}

func (i *InvalidDataDirectoryError) Is(err error) bool {
	return err == ErrInvalidDataDirectory
}

func VerifyDataDirectory(path string) error {
	var mErr multierror.Error
	for _, f := range PostgresFiles {
		if !PathExists(filepath.Join(path, f)) {
			mErr = *multierror.Append(&mErr, &InvalidDataDirectoryError{path, f})
		}
	}

	return mErr.ErrorOrNil()
}

func alreadyRenamed(archive, target string) bool {
	return PathExists(archive) && !PathExists(target)
}

func PathExists(path string) bool {
	_, err := utils.System.Stat(path)
	return err == nil
}

// Each directory in 'directories' is deleted only if every path in 'requiredPaths' exists
// in that directory.
func DeleteDirectories(directories []string, requiredPaths []string, streams step.OutStreams) error {
	var mErr *multierror.Error
	for _, directory := range directories {
		statError := false

		for _, requiredPath := range requiredPaths {
			filePath := filepath.Join(directory, requiredPath)
			_, err := utils.System.Stat(filePath)
			if err != nil {
				mErr = multierror.Append(mErr, err)
				statError = true
			}
		}

		if statError {
			continue
		}

		hostname, err := utils.System.Hostname()
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(streams.Stdout(), "Deleting directory: %q on host %q\n", directory, hostname)
		if err != nil {
			return err
		}

		err = utils.System.RemoveAll(directory)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}
