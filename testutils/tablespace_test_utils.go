// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func MustAddTablespace(t *testing.T, cluster greenplum.Cluster, tablespaceDir string) {
	t.Helper()

	if cluster.Version.Major == 5 {
		MustCreateFilespaceAndTablespace(t, cluster, tablespaceDir)
	}

	if cluster.Version.Major >= 6 {
		return
		//MustCreateTablespace(t, cluster, tablespaceDir)
	}

	// create databases in the tablespace
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), `CREATE DATABASE foodb TABLESPACE batsTbsp;`)
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), `CREATE DATABASE eatdb TABLESPACE batsTbsp;`)

	// create tables in the tablespace
	sql := `
CREATE TABLE public.tablespace_table_0 (a int) TABLESPACE batsTbsp;
INSERT INTO public.tablespace_table_0 SELECT i from generate_series(1,100)i;

CREATE TABLE public.tablespace_table_1 (a int) WITH(appendonly=true, orientation=row) TABLESPACE batsTbsp;
INSERT INTO public.tablespace_table_1 SELECT i from generate_series(1,100)i;

CREATE TABLE public.tablespace_table_2 (a int) WITH(appendonly=true, orientation=column) TABLESPACE batsTbsp;
INSERT INTO public.tablespace_table_2 SELECT i from generate_series(1,100)i;

CREATE TABLE  public.tablespace_table_3 (a int, b int) WITH(appendonly=true, orientation=column) TABLESPACE batsTbsp
PARTITION BY RANGE(b) (START(1) END(4) EVERY(1));
INSERT INTO public.tablespace_table_3 SELECT i, (i%3)+1 FROM generate_series(1,100)i;`
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), sql)

	// add a table to the database within the tablespace
	sql = `
CREATE TABLE public.tablespace_table_0 (a int);
INSERT INTO public.tablespace_table_0 SELECT i from generate_series(1,100)i;`
	conn := cluster.Connection(greenplum.Database("foodb"))
	MustExecuteSQL(t, conn, sql)

	// add a table to the database within the tablespace
	sql = `
CREATE TABLE public.tablespace_table_0 (a int);
INSERT INTO public.tablespace_table_0 SELECT i from generate_series(1,100)i;`
	conn = cluster.Connection(greenplum.Database("eatdb"))
	MustExecuteSQL(t, conn, sql)
}

func MustCreateTablespace(t *testing.T, cluster greenplum.Cluster, tablespaceDir string) {
	t.Helper()

	path := filepath.Join(tablespaceDir, "testfs")
	MustCreateDir(t, path)
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), fmt.Sprintf(`CREATE TABLESPACE batsTbsp LOCATION '%s';`, path))
}

func MustCreateFilespaceAndTablespace(t *testing.T, cluster greenplum.Cluster, tablespaceDir string) {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("filespace:batsFS\n")
	for _, seg := range cluster.Primaries {
		// Keep symlinks short otherwise they get trimmed and result in an invalid symlink when pg_basebackup copies
		// the coordinator to standby.
		path := filepath.Join(tablespaceDir, "testfs", strconv.Itoa(seg.DbID), filepath.Base(seg.DataDir))
		MustCreateDirRemotely(t, seg.Hostname, filepath.Dir(path))
		sb.WriteString(fmt.Sprintf("%s:%d:%s\n", seg.Hostname, seg.DbID, path))
	}

	for _, seg := range cluster.Mirrors {
		// Keep symlinks short otherwise they get trimmed and result in an invalid symlink when pg_basebackup copies
		// the coordinator to standby.
		path := filepath.Join(tablespaceDir, "testfs", strconv.Itoa(seg.DbID), filepath.Base(seg.DataDir))
		MustCreateDirRemotely(t, seg.Hostname, filepath.Dir(path))
		sb.WriteString(fmt.Sprintf("%s:%d:%s\n", seg.Hostname, seg.DbID, path))
	}

	config := filepath.Join(tablespaceDir, "testfs", "fs.config")
	MustWriteToFile(t, config, sb.String())

	// gpfilespace requires the HOME environment variable
	err := cluster.RunGreenplumCmdWithEnvironment(step.NewLogStdStreams(false), "gpfilespace", []string{"--config", config}, utils.FilterEnv([]string{"HOME"}))
	if err != nil {
		t.Fatal(err)
	}

	// create a tablespace in the filespace
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), `CREATE TABLESPACE batsTbsp FILESPACE batsFS;`)
}

func MustDeleteTablespaces(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	if cluster.Version.Major >= 6 {
		return
	}

	conn := cluster.Connection(greenplum.Database("foodb"))
	MustExecuteSQL(t, conn, `DROP TABLE IF EXISTS public.tablespace_table_0;`)

	conn = cluster.Connection(greenplum.Database("eatdb"))
	MustExecuteSQL(t, conn, `DROP TABLE IF EXISTS public.tablespace_table_0;`)

	MustExecuteSQL(t, cluster.Connection(), `DROP DATABASE foodb;`)
	MustExecuteSQL(t, cluster.Connection(), `DROP DATABASE eatdb;`)

	sql := `
DROP TABLE IF EXISTS public.tablespace_table_0;
DROP TABLE IF EXISTS public.tablespace_table_1;
DROP TABLE IF EXISTS public.tablespace_table_2;
DROP TABLE IF EXISTS public.tablespace_table_3;`
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), sql)

	sql = `DROP TABLESPACE IF EXISTS batsTbsp;`
	if cluster.Version.Major == 5 {
		sql += `DROP FILESPACE IF EXISTS batsFS;`
	}

	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), sql)
}

func MustTruncateTablespaces(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	if cluster.Version.Major >= 6 {
		return
	}

	sql := `
TRUNCATE public.tablespace_table_0;
TRUNCATE public.tablespace_table_1;
TRUNCATE public.tablespace_table_2;
TRUNCATE public.tablespace_table_3;`
	MustExecuteSQL(t, cluster.Connection(greenplum.Database("postgres")), sql)

	conn := cluster.Connection(greenplum.Database("foodb"))
	MustExecuteSQL(t, conn, `TRUNCATE public.tablespace_table_0;`)

	conn = cluster.Connection(greenplum.Database("eatdb"))
	MustExecuteSQL(t, conn, `TRUNCATE public.tablespace_table_0;`)
}

func VerifyTablespaceData(t *testing.T, cluster greenplum.Cluster) {
	t.Helper()

	if cluster.Version.Major >= 6 {
		return
	}

	tables := []string{"public.tablespace_table_0", "public.tablespace_table_1", "public.tablespace_table_2", "public.tablespace_table_3"}
	for _, table := range tables {
		rows := MustQueryRow(t, cluster.Connection(greenplum.Database("postgres")), `SELECT COUNT(*) FROM `+table)
		expected := 100
		if rows != expected {
			t.Fatalf("got %v want %v rows", rows, expected)
		}
	}

	conn := cluster.Connection(greenplum.Database("foodb"))
	row := MustQueryRow(t, conn, `SELECT COUNT(*) FROM public.tablespace_table_0;`)
	expectedCount := 100
	if row != expectedCount {
		t.Fatalf("got %d want %d rows", row, expectedCount)
	}

	conn = cluster.Connection(greenplum.Database("eatdb"))
	row = MustQueryRow(t, conn, `SELECT COUNT(*) FROM public.tablespace_table_0;`)
	expectedCount = 100
	if row != expectedCount {
		t.Fatalf("got %d want %d rows", row, expectedCount)
	}
}
