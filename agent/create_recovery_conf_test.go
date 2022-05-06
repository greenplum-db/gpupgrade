// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestCreateRecoveryConf(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("creates recovery.conf", func(t *testing.T) {
		mirrorDataDir := testutils.GetTempDir(t, "")

		connReqs := []*idl.CreateRecoveryConfRequest_Connection{{
			MirrorDataDir: mirrorDataDir,
			User:          "gpadmin",
			PrimaryHost:   "sdw1",
			PrimaryPort:   int32(123),
		}}

		_, err := server.CreateRecoveryConf(context.Background(), &idl.CreateRecoveryConfRequest{Connections: connReqs})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		contents := testutils.MustReadFile(t, filepath.Join(mirrorDataDir, "recovery.conf"))
		expected := `standby_mode = 'on'
primary_conninfo = 'user=gpadmin host=sdw1 port=123 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'`

		if contents != expected {
			t.Errorf("got %q, want %q", contents, expected)
		}
	})

	t.Run("returns multiple errors when failing to write recovery.conf", func(t *testing.T) {
		connReqs := []*idl.CreateRecoveryConfRequest_Connection{
			{
				MirrorDataDir: "/does/not/exist",
				User:          "gpadmin",
				PrimaryHost:   "sdw1",
				PrimaryPort:   int32(123),
			},
			{
				MirrorDataDir: "/also/does/not/exist",
				User:          "gpadmin",
				PrimaryHost:   "sdw2",
				PrimaryPort:   int32(456),
			}}

		_, err := server.CreateRecoveryConf(context.Background(), &idl.CreateRecoveryConfRequest{Connections: connReqs})
		if err == nil {
			t.Error("expected error, returned nil")
		}

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Errorf("got %d errors want 2", len(errs))
		}

		for _, err := range errs {
			var pathError *os.PathError
			if !errors.As(err, &pathError) {
				t.Errorf("got type %T want %T", err, pathError)
			}
		}
	})
}
