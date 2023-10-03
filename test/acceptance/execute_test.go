// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestExecute(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("gpupgrade execute should remember that link mode was specified in initialize", func(t *testing.T) {
		table := "public.test_linking"

		source := acceptance.GetSourceCluster(t)
		testutils.MustExecuteSQL(t, source.Connection(), fmt.Sprintf(`CREATE TABLE %s (a int);`, table))
		defer testutils.MustExecuteSQL(t, source.Connection(), fmt.Sprintf(`DROP TABLE IF EXISTS %s;`, table))

		sourceRelfilenodes := getRelfilenodes(t, source.Connection(), source.Version, table)
		for _, relfilenode := range sourceRelfilenodes {
			hardlinks := getNumHardLinks(t, relfilenode)
			if hardlinks != 1 {
				t.Fatalf("got %q want %q hardlinks", hardlinks, 1)
			}
		}

		acceptance.Initialize(t, idl.Mode_link)
		defer acceptance.Revert(t)

		acceptance.Execute(t)

		intermediate := acceptance.GetIntermediateCluster(t)
		intermediateRelfilenodes := getRelfilenodes(t, intermediate.Connection(), intermediate.Version, table)
		for _, relfilenode := range intermediateRelfilenodes {
			hardlinks := getNumHardLinks(t, relfilenode)
			if hardlinks != 2 {
				t.Fatalf("got %q want %q hardlinks", hardlinks, 2)
			}
		}
	})

	t.Run("gpupgrade execute step to upgrade coordinator should always rsync the coordinator data dir from backup", func(t *testing.T) {
		acceptance.Initialize(t, idl.Mode_link)
		defer acceptance.Revert(t)

		// For substep idempotence initialize creates a backup of the
		// intermediate coordinator data directory. During execute before
		// upgrading the coordinator the intermediate coordinator data directory
		// is refreshed with the backup. Remove the intermediate coordinator
		// data directory to ensure that initialize created the backup and
		// execute correctly utilizes it.
		intermediateCoordinatorDataDir := configShow(t, "--target-datadir")
		testutils.MustRemoveAll(t, intermediateCoordinatorDataDir)

		// create an extra file to ensure that it's deleted during rsync
		path := filepath.Join(intermediateCoordinatorDataDir, "base_extra")
		testutils.MustCreateDir(t, path)
		testutils.MustWriteToFile(t, filepath.Join(path, "1101"), "extra_relfilenode")

		acceptance.Execute(t)

		testutils.PathMustNotExist(t, path)
	})

	t.Run("all substeps can be re-run after completion", func(t *testing.T) {
		source := acceptance.GetSourceCluster(t)

		acceptance.Initialize(t, idl.Mode_copy)
		defer acceptance.Revert(t)

		acceptance.Execute(t)

		// undo the upgrade so that we can re-run execute
		err := source.Start(step.DevNullStream)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// see comment in revert.go on why we ignore gpstart failures
			if !(exitErr.ExitCode() == 1 && len(exitErr.Stderr) == 0 && source.Version.Major == 5) {
				t.Fatal(err)
			}
		}

		err = hub.Recoverseg(step.DevNullStream, &source, false)
		if err != nil {
			t.Fatal(err)
		}

		intermediate := acceptance.GetIntermediateCluster(t)
		err = intermediate.Stop(step.DevNullStream)
		if err != nil {
			t.Fatal(err)
		}

		// As a hacky way of testing substep idempotence mark all execute substeps as failed and re-run.
		replaced := acceptance.Jq(t, filepath.Join(utils.GetStateDir(), step.SubstepsFileName), `(.execute | values[]) |= "failed"`)
		testutils.MustWriteToFile(t, filepath.Join(utils.GetStateDir(), step.SubstepsFileName), replaced)

		acceptance.Execute(t)
	})
}

func getRelfilenodes(t *testing.T, connection string, version semver.Version, tableName string) []string {
	t.Helper()

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
	t.Helper()

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
