// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
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

		if pathExists(stateDir) {
			t.Fatal("expected stateDir to not exist")
		}

		go server.Start()
		defer server.Stop()

		exists, err := doesPathEventuallyExist(stateDir)
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

		if !pathExists(stateDir) {
			t.Fatal("expected stateDir to exist")
		}

		go server.Start()
		defer server.Stop()

		if !pathExists(stateDir) {
			t.Error("expected stateDir to exist")
		}
	})
}

func doesPathEventuallyExist(path string) (bool, error) {
	startTime := time.Now()
	timeout := 3 * time.Second

	for {
		exists := pathExists(path)
		if exists {
			return true, nil
		}

		if time.Since(startTime) > timeout {
			return false, xerrors.Errorf("timeout exceeded")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
