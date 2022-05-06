// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"os"
	"path"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestServerStart(t *testing.T) {
	testlog.SetupLogger()

	t.Run("successfully starts and creates state directory if it does not exist", func(t *testing.T) {
		tempDir := testutils.GetTempDir(t, "")
		defer os.RemoveAll(tempDir)
		stateDir := path.Join(tempDir, ".gpupgrade")

		server := agent.NewServer(agent.Config{
			Port:     testutils.MustGetPort(t),
			StateDir: stateDir,
		})

		testutils.PathMustNotExist(t, stateDir)

		go server.Start()
		defer server.Stop()

		exists, err := doesPathEventuallyExist(t, stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}
		if !exists {
			t.Error("expected stateDir to be created")
		}
	})

	t.Run("successfully starts if state directory already exists", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, ".gpupgrade")
		defer os.RemoveAll(stateDir)

		server := agent.NewServer(agent.Config{
			Port:     testutils.MustGetPort(t),
			StateDir: stateDir,
		})

		testutils.PathMustExist(t, stateDir)

		go server.Start()
		defer server.Stop()

		testutils.PathMustExist(t, stateDir)
	})
}

func doesPathEventuallyExist(t *testing.T, path string) (bool, error) {
	startTime := time.Now()
	timeout := 3 * time.Second

	for {
		exists, err := upgrade.PathExist(path)
		if err != nil {
			t.Fatalf("checking path %q: %v", path, err)
		}

		if exists {
			return true, nil
		}

		if time.Since(startTime) > timeout {
			return false, xerrors.Errorf("timeout exceeded")
		}

		time.Sleep(10 * time.Millisecond)
	}
}
