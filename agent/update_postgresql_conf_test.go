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

func TestServer_UpdatePostgresqlConf(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		opts := []*idl.UpdateFileConfOptions{
			{
				Path:         filepath.Join("", "postgresql.conf"),
				CurrentValue: 0,
				UpdatedValue: 0,
			},
			{
				Path:         filepath.Join("", "postgresql.conf"),
				CurrentValue: 0,
				UpdatedValue: 0,
			}}

		_, err := server.UpdatePostgresqlConf(context.Background(), &idl.UpdatePostgresqlConfRequest{Options: opts})

		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != len(opts) {
			t.Fatalf("got error count %d, want %d", len(errs), len(opts))
		}

		for _, err := range errs {
			expected := "update postgresql.conf"
			if !strings.HasPrefix(err.Error(), expected) {
				t.Errorf("expected error to contain %q got %q", expected, err.Error())
			}
		}
	})

	t.Run("succeeds", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		path := filepath.Join(dir, "postgresql.conf")
		testutils.MustWriteToFile(t, path, "port=123")

		opts := []*idl.UpdateFileConfOptions{{Path: path, CurrentValue: 123, UpdatedValue: 456}}
		_, err := server.UpdatePostgresqlConf(context.Background(), &idl.UpdatePostgresqlConfRequest{Options: opts})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		actual := testutils.MustReadFile(t, path)
		expected := "port=456"
		if !strings.HasPrefix(actual, expected) {
			t.Errorf("got %q want %q", actual, expected)
		}
	})
}
