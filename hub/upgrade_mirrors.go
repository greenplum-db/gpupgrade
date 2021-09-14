// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"time"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func UpgradeMirrors(streams step.OutStreams, conn *greenplum.Conn, intermediate *greenplum.Cluster, useHbaHostnames bool) (err error) {
	err = writeAddMirrorsConfig(intermediate)
	if err != nil {
		return err
	}

	err = runGpAddMirrors(greenplum.NewRunner(intermediate, streams), useHbaHostnames)
	if err != nil {
		return err
	}

	options := []greenplum.Option{
		greenplum.ToTarget(),
		greenplum.Port(intermediate.MasterPort()),
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

	return waitForFTS(db, 2*time.Minute)
}

func writeAddMirrorsConfig(intermediate *greenplum.Cluster) error {
	var config bytes.Buffer
	for _, m := range intermediate.Mirrors {
		if m.IsStandby() {
			continue
		}

		_, err := fmt.Fprintf(&config, "%d|%s|%d|%s\n", m.ContentID, m.Hostname, m.Port, m.DataDir)
		if err != nil {
			return err
		}
	}

	err := os.WriteFile(utils.GetAddMirrorsConfig(), config.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func runGpAddMirrors(targetRunner greenplum.Runner, useHbaHostnames bool) error {
	args := []string{"-a", "-i", utils.GetAddMirrorsConfig()}
	if useHbaHostnames {
		args = append(args, "--hba-hostnames")
	}

	err := targetRunner.Run("gpaddmirrors", args...)
	if err != nil {
		return err
	}

	return nil
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
