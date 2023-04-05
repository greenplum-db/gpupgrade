// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestServer_RestorePrimariesPgControl(t *testing.T) {
	testlog.SetupTestLogger()
	agentServer := agent.New()

	t.Run("bubbles up errors when no pg_control files exist", func(t *testing.T) {
		dirs := []string{"/tmp/test1", "/tmp/test2"}
		_, err := agentServer.RestorePrimariesPgControl(context.Background(), &idl.RestorePgControlRequest{Datadirs: dirs})

		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != len(dirs) {
			t.Fatalf("got error count %d, want %d", len(errs), len(dirs))
		}

		for i, err := range errs {
			if !os.IsNotExist(err) {
				t.Errorf("got error type %T, want %T", err, &os.LinkError{})
			}

			if !strings.Contains(err.(*os.LinkError).Error(), dirs[i]) {
				t.Errorf("got path %s, want %s", err.(*os.PathError).Path, dirs[i])
			}
		}
	})

	t.Run("finishes successfully", func(t *testing.T) {
		sourceDir := testutils.GetTempDir(t, "")

		globalDir := filepath.Join(sourceDir, "global")
		err := utils.System.Mkdir(globalDir, 0755)
		if err != nil {
			t.Fatalf("failed to create dir %s", globalDir)
		}

		src := filepath.Join(sourceDir, "global", "pg_control")
		testutils.MustWriteToFile(t, src, "")

		_, err = agentServer.RestorePrimariesPgControl(context.Background(), &idl.RestorePgControlRequest{Datadirs: []string{sourceDir}})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})
}
