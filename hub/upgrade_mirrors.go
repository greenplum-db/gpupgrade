// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const defaultFTSTimeout = 2 * time.Minute

func writeGpAddmirrorsConfig(mirrors []greenplum.SegConfig, out io.Writer) error {
	for _, m := range mirrors {
		_, err := fmt.Fprintf(out, "%d|%s|%d|%s\n", m.ContentID, m.Hostname, m.Port, m.DataDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func runAddMirrors(r greenplum.Runner, filepath string, useHbaHostnames bool) error {
	args := []string{"-a", "-i", filepath}
	if useHbaHostnames {
		args = append(args, "--hba-hostnames")
	}

	return r.Run("gpaddmirrors", args...)
}

func waitForFTS(db *sql.DB, timeout time.Duration) error {
	startTime := time.Now()
	for {
		rows, err := db.Query("SELECT gp_request_fts_probe_scan();")
		if err != nil {
			return xerrors.Errorf("requesting probe scan: %w", err)
		}

		if err := rows.Close(); err != nil {
			return xerrors.Errorf("closing probe scan results: %w", err)
		}

		doneWaiting, err := func() (bool, error) {
			var up bool
			rows, err = db.Query(`
				SELECT every(status = 'u' AND mode = 's')
					FROM gp_segment_configuration
					WHERE role = 'm'
			`)
			if err != nil {
				return false, xerrors.Errorf("querying mirror status: %w", err)
			}

			defer rows.Close() // XXX lost error

			for rows.Next() {
				if err := rows.Scan(&up); err != nil {
					return false, xerrors.Errorf("scanning mirror status: %w", err)
				}
			}
			if err := rows.Err(); err != nil {
				return false, xerrors.Errorf("iterating mirror status: %w", err)
			}

			return up, nil
		}()

		if err != nil {
			return err
		}

		if doneWaiting {
			return nil
		}

		if time.Since(startTime) > timeout {
			return xerrors.Errorf("%s timeout exceeded waiting for mirrors to come up", timeout)
		}

		time.Sleep(time.Second)
	}
}

func UpgradeMirrors(stateDir string, conn *greenplum.Conn, masterPort int, mirrors []greenplum.SegConfig, targetRunner greenplum.Runner, useHbaHostnames bool) (err error) {
	options := []greenplum.Option{
		greenplum.ToTarget(),
		greenplum.Port(masterPort),
		greenplum.UtilityMode(),
	}

	db, err := sql.Open("pgx", conn.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	return doUpgrade(db, stateDir, mirrors, targetRunner, useHbaHostnames)
}

func doUpgrade(db *sql.DB, stateDir string, mirrors []greenplum.SegConfig, targetRunner greenplum.Runner, useHbaHostnames bool) (err error) {
	path := filepath.Join(stateDir, "add_mirrors_config")
	// calling Close() on a file twice results in an error
	// only call Close() in the defer if we haven't yet tried to close it.
	fileClosed := false

	f, err := utils.System.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if !fileClosed {
			if cerr := f.Close(); cerr != nil {
				err = errorlist.Append(err, cerr)
			}
		}
	}()

	err = writeGpAddmirrorsConfig(mirrors, f)
	if err != nil {
		return err
	}

	err = f.Close()
	fileClosed = true
	// not unit tested because stubbing it properly
	// would require too many extra layers
	if err != nil {
		return err
	}

	err = runAddMirrors(targetRunner, path, useHbaHostnames)
	if err != nil {
		return err
	}

	return waitForFTS(db, defaultFTSTimeout)
}
