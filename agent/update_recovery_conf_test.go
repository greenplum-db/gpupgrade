// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestServer_UpdateRecoveryConf(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		opts := []*idl.UpdateFileConfOptions{
			{
				Path:    filepath.Join("", "recovery.conf"),
				OldPort: 0,
				NewPort: 0,
			},
			{
				Path:    filepath.Join("", "recovery.conf"),
				OldPort: 0,
				NewPort: 0,
			}}

		_, err := server.UpdateRecoveryConf(context.Background(), &idl.UpdateRecoveryConfRequest{Options: opts})

		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != len(opts) {
			t.Fatalf("got error count %d, want %d", len(errs), len(opts))
		}

		for _, err := range errs {
			expected := "update recovery.conf"
			if !strings.HasPrefix(err.Error(), expected) {
				t.Errorf("expected error to contain %q", expected)
			}
		}
	})

	t.Run("succeeds", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		path := filepath.Join(dir, "recovery.conf")
		testutils.MustWriteToFile(t, path, "primary_conninfo = 'user=gpadmin host=sdw1 port=123 sslmode=disable")

		opts := []*idl.UpdateFileConfOptions{{Path: path, OldPort: 123, NewPort: 456}}
		_, err := server.UpdateRecoveryConf(context.Background(), &idl.UpdateRecoveryConfRequest{Options: opts})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		actual := testutils.MustReadFile(t, path)
		expected := "primary_conninfo = 'user=gpadmin host=sdw1 port=456 sslmode=disable"
		if !strings.HasPrefix(actual, expected) {
			t.Errorf("got %q want %q", actual, expected)
		}
	})
}
