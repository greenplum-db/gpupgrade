//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestAddReplicationEntriesToPgHbaConf(t *testing.T) {
	t.Run("succeeds in appending replication entries to file", func(t *testing.T) {
		dir1 := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir1)
		path1 := filepath.Join(dir1, "pg_hba.conf")
		testutils.MustWriteToFile(t, path1, "some existing text\n")

		dir2 := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir2)
		path2 := filepath.Join(dir2, "pg_hba.conf")
		testutils.MustWriteToFile(t, path2, "some more existing text in the other file\n")

		confs := []*idl.AddReplicationEntriesRequest_Entry{
			{
				DataDir:   dir1,
				User:      "gpadmin",
				HostAddrs: []string{"127.0.0.1", "127.0.0.2"},
			},
			{
				DataDir:   dir2,
				User:      "gpadmin",
				HostAddrs: []string{"10.0.0.3", "10.0.0.4"},
			},
		}

		err := agent.AddReplicationEntriesToPgHbaConf(confs)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		contents := testutils.MustReadFile(t, path1)
		expected := `some existing text
host replication gpadmin samehost trust
host all gpadmin 127.0.0.1 trust
host replication gpadmin 127.0.0.1 trust
host all gpadmin 127.0.0.2 trust
host replication gpadmin 127.0.0.2 trust
`
		if contents != expected {
			t.Errorf("got %q, want %q", contents, expected)
		}

		contents = testutils.MustReadFile(t, path2)
		expected = `some more existing text in the other file
host replication gpadmin samehost trust
host all gpadmin 10.0.0.3 trust
host replication gpadmin 10.0.0.3 trust
host all gpadmin 10.0.0.4 trust
host replication gpadmin 10.0.0.4 trust
`
		if contents != expected {
			t.Errorf("got %q, want %q", contents, expected)
		}
	})

	t.Run("errors when failing to open file", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer func() {
			err := os.Chmod(dir, 0700)
			if err != nil {
				t.Fatalf("making tablespace location directory writeable: %v", err)
			}
			testutils.MustRemoveAll(t, dir)
		}()

		path := filepath.Join(dir, "pg_hba.conf")
		testutils.MustWriteToFile(t, path, "some existing text\n")

		// Set file to read only so writing to it fails
		err := os.Chmod(path, 0500)
		if err != nil {
			t.Fatalf("making directory %q read only: %v", dir, err)
		}

		err = agent.AddReplicationEntriesToPgHbaConf([]*idl.AddReplicationEntriesRequest_Entry{{DataDir: dir, User: "", HostAddrs: []string{""}}})
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("got error %#v want %#v", err, os.ErrPermission)
		}
	})
}
