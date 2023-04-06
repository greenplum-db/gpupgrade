// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const OldSuffix = ".old"
const PGVersion = "PG_VERSION"

var PostgresFiles = []string{"postgresql.conf", PGVersion}
var StateDirectoryFiles = []string{"config.json", step.SubstepsFileName}

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

// RenameDirectories archives the source directory, and renames
// source to target. For example:
// source '/data/dbfast1/demoDataDir0' becomes archive '/data/dbfast1/demoDataDir.123ABC.0.old'
// target '/data/dbfast1/demoDataDir.123ABC.0' becomes source '/data/dbfast1/demoDataDir0'
func RenameDirectories(source, target string) error {
	// Instead of manipulating the source to create the archive we append the
	// old suffix to the target to achieve the same result.
	archive := target + OldSuffix

	alreadyRenamed, err := AlreadyRenamed(target, archive)
	if err != nil {
		return err
	}

	if alreadyRenamed {
		return nil
	}

	sourceExist, err := PathExist(source)
	if err != nil {
		return err
	}

	if sourceExist {
		if err := renameDataDirectory(source, archive); err != nil {
			return err
		}
	}

	if !sourceExist {
		log.Printf("Source directory not found when renaming %q to %q. It was already renamed from a previous run.", source, archive)
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

func VerifyDataDirectory(path ...string) error {
	var err error

	for _, f := range PostgresFiles {
		for _, p := range path {
			exist, pErr := PathExist(filepath.Join(p, f))
			if pErr != nil {
				err = errorlist.Append(err, pErr)
			}

			if !exist {
				err = errorlist.Append(err, &InvalidDataDirectoryError{p, f})
			}
		}
	}

	return err
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

func PathExistInFS(fsys fs.FS, path string) (bool, error) {
	_, err := utils.System.StatFS(fsys, path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
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
	var mErr error

	for _, f := range files {
		path := filepath.Join(path, f)
		_, err := utils.System.Stat(path)
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}

	return mErr
}

// Each directory in 'directories' is deleted only if every path in 'requiredPaths' exists
// in that directory.
func DeleteDirectories(directories []string, requiredPaths []string, streams step.OutStreams) error {
	hostname, err := utils.System.Hostname()
	if err != nil {
		return err
	}

	var mErr error
	for _, directory := range directories {
		log.Printf("Deleting directory: %q on host %q\n", directory, hostname)
		_, err = fmt.Fprintf(streams.Stdout(), "Deleting directory: %q on host %q\n", directory, hostname)
		if err != nil {
			return err
		}

		directoryExist, err := PathExist(directory)
		if err != nil {
			return err
		}

		if !directoryExist {
			fmt.Fprintf(streams.Stdout(), "directory: %q does not exist on host %q\n", directory, hostname)
			continue
		}

		err = verifyPathsExist(directory, requiredPaths...)
		if err != nil {
			mErr = errorlist.Append(mErr, err)
			continue
		}

		err = utils.System.RemoveAll(directory)
		if err != nil {
			mErr = errorlist.Append(mErr, err)
		}
	}

	return mErr
}
