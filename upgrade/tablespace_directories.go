//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TablespacePath(tablespaceLocation string, dbID int, majorVersion uint64, catalogVersion string) string {
	return filepath.Join(
		tablespaceLocation,
		strconv.Itoa(dbID),
		fmt.Sprintf("GPDB_%d_%s", majorVersion, catalogVersion),
	)
}

// DeleteTablespaceDirectories deletes tablespace directories with the
// following format:
//   /dir/<fsname>/<datadir>/<tablespaceOid>/<dbId>/GPDB_<majorVersion>_<catalogVersion>
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
//  GPDB 5X:  /dir/<fsname>/<datadir>/<tablespaceOID>/<dbOID>/<relfilenode>
//  GPDB 6X:  /dir/<fsname>/<datadir>/<tablespaceOID>/<dbID>/GPDB_6_<catalogVersion>/<dbOID>/<relfilenode>
func DeleteTablespaceDirectories(streams step.OutStreams, dirs []string) error {
	for _, dir := range dirs {
		exist, err := VerifyTablespaceDirectory(filepath.Dir(dir))
		if err != nil && errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return err
		}

		if !exist {
			return xerrors.Errorf("wat")
		}
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

// VerifyTablespaceDirectories takes in list of tablespace locations and verifies
// that they are either tablespace directories or legacy tablespace directories.
// It takes an input path of /dir/<fsname>/<datadir>/<tablespaceOID>
func VerifyTablespaceDirectories(tsLocations []string) error {
	var mErr error
	for _, tsLocation := range tsLocations {
		entries, err := ioutil.ReadDir(tsLocation)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			return xerrors.Errorf("Invalid tablespace directory %q", tsLocation)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			path := filepath.Join(tsLocation, entry.Name())
			exist_5X, err := VerifyLegacyTablespaceDirectory(path)
			if err != nil {
				mErr = errorlist.Append(mErr, err)
				continue
			}

			if exist_5X {
				continue
			}

			exist_6X, err := VerifyTablespaceDirectory(path)
			if err != nil {
				mErr = errorlist.Append(mErr, err)
				continue
			}

			if !exist_6X {
				mErr = errorlist.Append(mErr, xerrors.Errorf("Invalid tablespace directory %q", path))
				continue
			}
		}
	}

	return mErr
}

// VerifyLegacyTablespaceDirectory verifies directories for GPDB 5X and lower.
// It takes an input path of /dir/<fsname>/<datadir>/<tablespaceOID>/<dbOID>
// and checks if the underlying relfilenode directory contains the PG_VERSION
// file. Note that the input path is one level down from the tablespace location.
// No error is returned when the dbOid directory does not exist since
// the user may not have created a table within the tablespace.
// The expected tablespace directory layout is:
//   /dir/<fsname>/<datadir>/<tablespaceOID>/<dbOID>/<relfilenode>
func VerifyLegacyTablespaceDirectory(dbOIDPath string) (bool, error) {
	path := filepath.Join(dbOIDPath, PGVersion)
	exist, err := PathExist(path)
	if err != nil {
		return false, xerrors.Errorf("checking legacy tablespace directory %q: %w", path, err)
	}

	if !exist {
		return false, nil
	}

	return true, nil
}

// VerifyTablespaceDirectory verifies directories for GPDB 6X and higher. It
// takes an input path of the form /dir/<fsname>/<datadir>/<tablespaceOID>/<dbID>
// and checks if the underlying directory starts with "GPDB_". Note that the
// input path is one level down from the tablespace location.
// The expected tablespace directory layout is:
//   /dir/<fsname>/<datadir>/<tablespaceOID>/<dbID>/GPDB_6_<catalogVersion>/<dbOID>/<relfilenode>
func VerifyTablespaceDirectory(path string) (bool, error) {
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return false, xerrors.Errorf("read tablespace directory %q: %w", path, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), "GPDB_") {
			return false, xerrors.Errorf("Invalid tablespace directory. Expected %q to start with 'GPDB_'.", filepath.Join(path, entry.Name()))
		}

		return true, nil
	}

	return false, nil
}
