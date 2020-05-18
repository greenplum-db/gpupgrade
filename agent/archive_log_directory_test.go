// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestArchiveLogDirectories(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		expected := errors.New("permission denied")

		utils.System.Rename = func(oldPath, newPath string) error {
			return expected
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		_, err := server.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{})
		if !xerrors.Is(err, expected) {
			t.Errorf("returned error %#v, want %#v", err, expected)
		}
	})

	t.Run("archives log directories", func(t *testing.T) {
		oldLogDir, _ := utils.GetLogDir()
		newLogDir := "/home/gpAdmin/newlogdir"
		calls := 0

		utils.System.Rename = func(oldPath, newPath string) error {
			calls++

			if oldPath != oldLogDir {
				t.Errorf("got %q want %q", oldPath, oldLogDir)
			}

			if newPath != newLogDir {
				t.Errorf("got %q want %q", newPath, newLogDir)
			}

			return nil
		}
		defer func() {
			utils.System.Rename = os.Rename
		}()

		_, err := server.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{NewDir: newLogDir})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if calls != 1 {
			t.Errorf("expected rename to be called once, got %d", calls)
		}
	})
}
