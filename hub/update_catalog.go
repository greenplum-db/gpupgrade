// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func UpdateCatalog(conn *greenplum.Conn, intermediate *greenplum.Cluster, target *greenplum.Cluster) error {
	options := []greenplum.Option{
		greenplum.ToTarget(),
		greenplum.Port(intermediate.MasterPort()),
		greenplum.UtilityMode(),
		greenplum.AllowSystemTableMods(),
	}

	db, err := sql.Open("pgx", conn.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			err = errorlist.Append(err, cerr)
		}
	}()

	return UpdateGpSegmentConfiguration(db, target)
}

func UpdateGpSegmentConfiguration(db *sql.DB, target *greenplum.Cluster) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() {
		err = commitOrRollback(tx, err)
	}()

	for _, seg := range target.Primaries {
		if err := updateSegment(tx, seg); err != nil {
			return err
		}
	}

	for _, seg := range target.Mirrors {
		if err := updateSegment(tx, seg); err != nil {
			return err
		}
	}

	return nil
}

func updateSegment(tx *sql.Tx, seg greenplum.SegConfig) error {
	result, err := tx.Exec("UPDATE gp_segment_configuration SET port = $1, datadir = $2 WHERE content = $3 AND role = $4", seg.Port, seg.DataDir, seg.ContentID, seg.Role)
	if err != nil {
		return xerrors.Errorf("update gp_segment_configuration: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		panic(fmt.Sprintf("retrieve rows affected: %v", err))
	}

	if rows != 1 {
		return xerrors.Errorf("Expected 1 row to be updated for segment dbid %d, but updated %d rows instead.", seg.DbID, rows)
	}

	return nil
}

// commitOrRollback either Commit()s or Rollback()s the passed transaction
// depending on whether err is non-nil. It returns any error encountered during
// the operation; in the case of a rollback error, the incoming error will be
// combined with the new error.
func commitOrRollback(tx *sql.Tx, err error) error {
	if err != nil {
		if rErr := tx.Rollback(); rErr != nil {
			rErr = xerrors.Errorf("roll back transaction: %w", rErr)
			err = errorlist.Append(err, rErr)
		}

		return err
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	return nil
}
