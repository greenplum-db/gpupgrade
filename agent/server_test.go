// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"fmt"
	"net"
	"path"
	"strings"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

const timeout = 1 * time.Second

func TestServerStart(t *testing.T) {
	testlog.SetupTestLogger()

	t.Run("successfully starts and creates state directory if it does not exist", func(t *testing.T) {
		tempDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, tempDir)
		stateDir := path.Join(tempDir, ".gpupgrade")

		agentServer := agent.New()

		testutils.PathMustNotExist(t, stateDir)

		errChan := make(chan error, 1)
		go func() {
			errChan <- agentServer.Start(testutils.MustGetPort(t), stateDir, false)
		}()

		exists, err := doesPathEventuallyExist(t, stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}

		if !exists {
			t.Error("expected stateDir to be created")
		}

		select {
		case err := <-errChan:
			if err != nil {
				t.Fatalf("unexpected error: %#v", err)
			}
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		default:
			agentServer.Stop()
		}
	})

	t.Run("successfully starts if state directory already exists", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, ".gpupgrade")
		defer testutils.MustRemoveAll(t, stateDir)

		agentServer := agent.New()

		testutils.PathMustExist(t, stateDir)

		errChan := make(chan error, 1)
		go func() {
			errChan <- agentServer.Start(testutils.MustGetPort(t), stateDir, false)
		}()

		testutils.PathMustExist(t, stateDir)

		select {
		case err := <-errChan:
			if err != nil {
				t.Fatalf("unexpected error: %#v", err)
			}
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		default:
			agentServer.Stop()
		}
	})

	t.Run("start returns an error when port is in use", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, ".gpupgrade")
		defer testutils.MustRemoveAll(t, stateDir)

		portInUse, closeListener := mustListen(t)
		defer closeListener()

		agentServer := agent.New()

		errChan := make(chan error, 1)
		go func() {
			errChan <- agentServer.Start(portInUse, stateDir, false)
		}()

		select {
		case err := <-errChan:
			expected := fmt.Sprintf("listen on port %d: listen tcp :%d: bind: address already in use", portInUse, portInUse)
			if err != nil && !strings.Contains(err.Error(), expected) {
				t.Errorf("got error %#v want %#v", err, expected)
			}
		case <-time.After(timeout): // use timeout to prevent test from hanging
			t.Error("timeout exceeded")
		}
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

func mustListen(t *testing.T) (int, func()) {
	t.Helper()

	listener, closeListener := getTcpListener(t)
	port := listener.Addr().(*net.TCPAddr).Port

	return port, closeListener
}

// getTcpListener returns a net.Listener and a function to close the listener
// for use in a defer.
func getTcpListener(t *testing.T) (net.Listener, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Errorf("unexpected error: %#v", err)
	}

	closeListener := func() {
		err := listener.Close()
		if err != nil {
			t.Fatalf("closing listener %#v", err)
		}
	}

	return listener, closeListener
}
