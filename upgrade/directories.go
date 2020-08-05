// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

const ConfigFileName = "config.json"
const OldSuffix = ".old"
const PGVersion = "PG_VERSION"

var PostgresFiles = []string{"postgresql.conf", PGVersion}
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

// TODO: Remove alreadyRenamed and use AlreadyRenamed
func alreadyRenamed(archive, target string) bool {
	return PathExists(archive) && !PathExists(target)
}

// AlreadyRenamed infers if a successful rename has already occurred
// by making sure src does not exist but dst does.
func AlreadyRenamed(src, dst string) (bool, error) {
	srcExist, err := PathExist(src)
	if err != nil {
		return false, err
	}

	dstExist, err := PathExist(dst)
	if err != nil {
		return false, err
	}

	return !srcExist && dstExist, nil
}

// TODO: Remove PathExists and use PathExist
func PathExists(path string) bool {
	_, err := utils.System.Stat(path)
	return err == nil
}

func PathExist(path string) (bool, error) {
	_, err := utils.System.Stat(path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func verifyPathsExist(path string, files ...string) error {
	var mErr *multierror.Error

	for _, f := range files {
		path := filepath.Join(path, f)
		_, err := utils.System.Stat(path)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// Each directory in 'directories' is deleted only if every path in 'requiredPaths' exists
// in that directory.
func DeleteDirectories(directories []string, requiredPaths []string, streams step.OutStreams) error {
	hostname, err := utils.System.Hostname()
	if err != nil {
		return err
	}

	var mErr *multierror.Error
	for _, directory := range directories {
		gplog.Debug("Deleting directory: %q on host %q\n", directory, hostname)
		_, err = fmt.Fprintf(streams.Stdout(), "Deleting directory: %q on host %q\n", directory, hostname)
		if err != nil {
			return err
		}

		if !PathExists(directory) {
			fmt.Fprintf(streams.Stdout(), "directory: %q does not exist on host %q\n", directory, hostname)
			gplog.Debug("Directory: %q does not exist on host %q\n", directory, hostname)
			continue
		}

		err = verifyPathsExist(directory, requiredPaths...)
		if err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}

		err = utils.System.RemoveAll(directory)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

var ErrInvalidTablespaceDirectory = errors.New("invalid tablespace directory")

// TablespaceDirectoryError is the backing error type for ErrInvalidTablespaceDirectory.
type TablespaceDirectoryError struct {
	description string
	reason      string
}

func newTablespaceDirectoryError(clusterDescription, reason string) *TablespaceDirectoryError {
	return &TablespaceDirectoryError{description: clusterDescription, reason: reason}
}

func (i *TablespaceDirectoryError) Error() string {
	return fmt.Sprintf("invalid %s tablespace directory: %s", i.description, i.reason)
}

func (i *TablespaceDirectoryError) Is(err error) bool {
	return err == ErrInvalidTablespaceDirectory
}

// DeleteNewTablespaceDirectories deletes tablespace directories with the
// following format:
//   DIR/<fsname>/<datadir>/<tablespaceOid>/<dbId>/GPDB_<majorVersion>_<catalogVersion>
// It first deletes the specified directory and checks if its safe to delete the
// parent dbID directory.
//
// Tablespace Directory Structure When Upgrading from 5X
// =====================================================
//
//   DIR
//   ├── filespace.txt
//   ├── master
//   │   ├── demoDataDir-1
//   │   │   └── 16385
//   │   │       ├── 1
//   │   │       │   └── GPDB_6_301908232
//   │   │       │       └── 12812
//   │   │       │           └── 16389
//   │   │       └── 12094
//   │   │           ├── 16384
//   │   │           └── PG_VERSION
//   ├── primary1
//   │   └── demoDataDir0
//   │       └── 16385
//   │           ├── 12094
//   │           │   ├── 16384
//   │           │   └── PG_VERSION
//   │           └── 2
//   │               └── GPDB_6_301908232
//   │                   └── 12812
//   │                       └── 16389
//
//  GPDB 5X:  DIR/<fsname>/<datadir>/<tablespaceOID>/<dbOID>/<relfilenode>
//  GPDB 6X:  DIR/<fsname>/<datadir>/<tablespaceOID>/<dbID>/GPDB_6_<catalogVersion>/<dbOID>/<relfilenode>
func DeleteNewTablespaceDirectories(streams step.OutStreams, dirs []string) error {
	if err := VerifyTargetTablespaceDirectories(dirs); err != nil {
		return err
	}

	err := DeleteDirectories(dirs, []string{}, streams)
	if err != nil {
		return err
	}

	// For example, the 6X tablespace
	//    /filespace/demoDataDir0/16386/1/GPDB_6_301908232
	// has been deleted above. Now check that its parent directory
	// can also be deleted by ensuring that its contents do not overlap with
	// the tablespace of 5X.
	for _, dir := range dirs {
		parent := filepath.Dir(filepath.Clean(dir))

		entries, err := ioutil.ReadDir(parent)
		if os.IsNotExist(err) {
			// directory may have been already removed during previous execution
			continue
		} else if err != nil {
			return err
		}

		// If the parent directory is not empty it contains files for the 5X
		// tablespace. For example, the oid for template1 is 1 which can conflict
		// with the 6X tablespace directory which uses segment dbid's which is
		// also 1. Thus, we do not want to delete the directory.
		if len(entries) > 0 {
			return nil
		}

		// If the directory is empty it 'only' contained the target cluster
		// tablespace and is safe to delete.
		// NOTE: Each directory passed in has a different parent.
		if err := os.Remove(parent); err != nil {
			return err
		}
	}

	return nil
}

// VerifyTargetTablespaceDirectories checks tablespace directories on GPDB 6X
// and later clusters.
func VerifyTargetTablespaceDirectories(dirs []string) error {
	for _, dir := range dirs {
		element := filepath.Base(dir)
		if !strings.HasPrefix(element, "GPDB_") {
			return newTablespaceDirectoryError("target", dir+` missing "GPDB_" in last path element`)
		}
	}

	return nil
}

// Verify5XTablespaceDirectories checks tablespace location directories of the
// following format: DIR/<fsname>/<datadir>/<tablespaceOID>
// It ensures the PG_VERSION file is found in all dbOid directories.
// NOTE: No error is returned when the dbOid directory does not exist since
// the user may not have created a table within the tablespace.
func Verify5XTablespaceDirectories(tsLocations []string) error {
	var mErr *multierror.Error
	for _, tsLocation := range tsLocations {
		entries, err := ioutil.ReadDir(tsLocation)
		if err != nil {
			return xerrors.Errorf("reading 5X tablespace directory: %w", err)
		}

		for _, dbOidDir := range entries {
			if !dbOidDir.IsDir() {
				continue
			}

			path := filepath.Join(tsLocation, dbOidDir.Name(), PGVersion)
			if !PathExists(path) {
				mErr = multierror.Append(mErr, newTablespaceDirectoryError("5X source cluster", "missing "+path))
			}
		}
	}

	return mErr.ErrorOrNil()
}

func TablespacePath(tablespaceLocation string, dbID int, majorVersion uint64, catalogVersion string) string {
	return filepath.Join(
		tablespaceLocation,
		strconv.Itoa(dbID),
		fmt.Sprintf("GPDB_%d_%s", majorVersion, catalogVersion),
	)
}
