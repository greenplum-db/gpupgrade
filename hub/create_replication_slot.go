// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"

	"golang.org/x/xerrors"
)

func CreateReplicationSlots(db *sql.DB) error {
	var slots int
	row := db.QueryRow(`SELECT COUNT(slot_name) FROM gp_dist_random('pg_replication_slots') WHERE slot_name = 'internal_wal_replication_slot';`)
	if err := row.Scan(&slots); err != nil {
		if err != nil && err != sql.ErrNoRows {
			return xerrors.Errorf("querying pg_replication_slots: %w", err)
		}
	}

	if slots > 0 {
		var result string
		row = db.QueryRow(`SELECT pg_drop_replication_slot('internal_wal_replication_slot') from gp_dist_random('gp_id');`)
		if err := row.Scan(&result); err != nil {
			if err == sql.ErrNoRows || err != nil {
				return xerrors.Errorf("pg_drop_replication_slot: %w", err)
			}
		}
	}

	var result string
	row = db.QueryRow(`SELECT pg_create_physical_replication_slot('internal_wal_replication_slot') from gp_dist_random('gp_id');`)
	if err := row.Scan(&result); err != nil {
		if err == sql.ErrNoRows || err != nil {
			return xerrors.Errorf("pg_create_physical_replication_slot: %w", err)
		}
	}

	return nil
}
