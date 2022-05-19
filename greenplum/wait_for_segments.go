// Copyright (c) 2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"
)

func WaitForSegments(db *sql.DB, timeout time.Duration, cluster *Cluster) error {
	startTime := time.Now()
	for {
		if cluster.Version.Major > 5 {
			rows, err := db.Query("SELECT gp_request_fts_probe_scan();")
			if err != nil {
				return xerrors.Errorf("requesting gp_request_fts_probe_scan: %w", err)
			}

			if err := rows.Close(); err != nil {
				return xerrors.Errorf("closing rows for gp_request_fts_probe_scan: %w", err)
			}
		}

		ready, err := areSegmentsReady(db, cluster)
		if err != nil {
			return err
		}

		if ready {
			return nil
		}

		if time.Since(startTime) > timeout {
			return xerrors.Errorf("%s timeout exceeded waiting for all segments to be up, in their preferred roles, and synchronized.", timeout)
		}

		time.Sleep(time.Second)
	}
}

func areSegmentsReady(db *sql.DB, cluster *Cluster) (bool, error) {
	var segments int

	// check gp_segment_configuration on segments
	whereClause := "AND mode = 's'"
	if !cluster.HasMirrors() {
		whereClause = ""
	}

	row := db.QueryRow(`SELECT COUNT(*) FROM gp_segment_configuration 
WHERE content > -1 AND status = 'u' AND (role = preferred_role) ` + whereClause)

	if err := row.Scan(&segments); err != nil {
		if err == sql.ErrNoRows {
			gplog.Debug("no rows found when querying gp_segment_configuration")
			return false, nil
		}

		return false, xerrors.Errorf("querying gp_segment_configuration: %w", err)
	}

	if segments != len(cluster.ExcludingCoordinatorOrStandby()) {
		return false, nil
	}

	// check gp_stat_replication for the standby. Note, gp_stat_replication does not exist in GPDB 5.
	if cluster.Version.Major == 5 || !cluster.HasStandby() {
		return true, nil
	}

	row = db.QueryRow("SELECT COUNT(*) FROM gp_stat_replication WHERE gp_segment_id = -1 AND state = 'streaming' AND sent_location = flush_location;")
	if err := row.Scan(&segments); err != nil {
		if err == sql.ErrNoRows {
			gplog.Debug("no rows found when querying gp_stat_replication")
			return false, nil
		}

		return false, xerrors.Errorf("querying gp_stat_replication: %w", err)
	}

	if segments != 1 {
		return false, nil
	}

	return true, nil
}
