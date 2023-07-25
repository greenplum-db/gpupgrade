// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"database/sql"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestExecute(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("gpupgrade execute should remember that link mode was specified in initialize", func(t *testing.T) {
		table := "public.test_linking"

		source := GetSourceCluster(t)
		executeSQL(t, source.Connection(), fmt.Sprintf(`CREATE TABLE %s (a int);`, table))
		defer executeSQL(t, source.Connection(), fmt.Sprintf(`DROP TABLE IF EXISTS %s;`, table))

		sourceRelfilenodes := getRelfilenodes(t, source.Connection(), source.Version, table)
		for _, relfilenode := range sourceRelfilenodes {
			hardlinks := getNumHardLinks(t, relfilenode)
			if hardlinks != 1 {
				t.Fatalf("got %q want %q hardlinks", hardlinks, 1)
			}
		}

		initialize(t, idl.Mode_link)
		execute(t)
		defer revert(t)

		intermediate := GetIntermediateCluster(t)
		intermediateRelfilenodes := getRelfilenodes(t, intermediate.Connection(), intermediate.Version, table)
		for _, relfilenode := range intermediateRelfilenodes {
			hardlinks := getNumHardLinks(t, relfilenode)
			if hardlinks != 2 {
				t.Fatalf("got %q want %q hardlinks", hardlinks, 2)
			}
		}
	})
}

func getRelfilenodes(t *testing.T, connection string, version semver.Version, tableName string) []string {
	db, err := sql.Open("pgx", connection)
	if err != nil {
		t.Fatalf("opening sql connection %q: %v", connection, err)
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	var query string
	if version.Major >= 6 {
		// Multiple db.Exec() calls are needed to create the helper functions since
		// doing so in a single db.Query call fails with:
		// `ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)`
		query = `
	CREATE FUNCTION pg_temp.seg_relation_filepath(tbl text)
        RETURNS TABLE (dbid int, path text)
        EXECUTE ON ALL SEGMENTS
        LANGUAGE SQL
    AS $$
        SELECT current_setting('gp_dbid')::int, pg_relation_filepath(tbl);
    $$;`
		_, err = db.Exec(query)
		if err != nil {
			t.Fatalf("executing sql %q: %v", query, err)
		}

		query = `
CREATE FUNCTION pg_temp.gp_relation_filepath(tbl text)
        RETURNS TABLE (dbid int, path text)
        LANGUAGE SQL
    AS $$
        SELECT current_setting('gp_dbid')::int, pg_relation_filepath(tbl)
            UNION ALL SELECT * FROM pg_temp.seg_relation_filepath(tbl);
    $$;`
		_, err = db.Exec(query)
		if err != nil {
			t.Fatalf("executing sql %q: %v", query, err)
		}

		query = fmt.Sprintf(`
    SELECT c.datadir || '/' || f.path
      FROM pg_temp.gp_relation_filepath('%s') f
      JOIN gp_segment_configuration c
        ON c.dbid = f.dbid;`, tableName)
	}

	if version.Major == 5 {
		query = fmt.Sprintf(`
 		SELECT e.fselocation||'/'||'base'||'/'||d.oid||'/'||c.relfilenode
          FROM gp_segment_configuration s
          JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
          JOIN pg_filespace f ON e.fsefsoid = f.oid
          JOIN pg_database d ON d.datname=current_database()
          JOIN gp_dist_random('pg_class') c ON c.gp_segment_id = s.content
        WHERE f.fsname = 'pg_system' AND role = 'p'
              AND c.relname = '%s'
        UNION ALL
        SELECT e.fselocation||'/'||'base'||'/'||d.oid||'/'||c.relfilenode
          FROM gp_segment_configuration s
          JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
          JOIN pg_filespace f ON e.fsefsoid = f.oid
          JOIN pg_database d ON d.datname=current_database()
          JOIN pg_class c ON c.gp_segment_id = s.content
        WHERE f.fsname = 'pg_system' AND role = 'p'
        AND c.relname = '%s';`, tableName, tableName)
	}

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("querying sql failed: %v", err)
	}
	defer rows.Close()

	var relfilenodes []string
	for rows.Next() {
		var relfilenode string
		err = rows.Scan(&relfilenode)
		if err != nil {
			t.Fatalf("scanning rows: %v", err)
		}

		relfilenodes = append(relfilenodes, relfilenode)
	}

	err = rows.Err()
	if err != nil {
		t.Fatalf("reading rows: %v", err)
	}

	return relfilenodes
}

func getNumHardLinks(t *testing.T, relfilenode string) uint64 {
	fileInfo, err := os.Stat(relfilenode)
	if err != nil {
		t.Fatalf("os.stat: %v", err)
	}

	hardLinks := uint64(0)
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		hardLinks = stat.Nlink
	}

	return hardLinks
}
