// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func gpupgrade_agent() {
}

func gpupgrade_agent_Errors() {
	os.Stderr.WriteString("could not find state-directory")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		gpupgrade_agent,
		gpupgrade_agent_Errors,
	)
}

func TestRestartAgent(t *testing.T) {
	testhelper.SetupTestLogger()
	listener := bufconn.Listen(1024 * 1024)
	agentServer := grpc.NewServer()
	defer agentServer.Stop()

	idl.RegisterAgentServer(agentServer, &agent.Server{})
	go func() {
		if err := agentServer.Serve(listener); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()

	hostnames := []string{"host1", "host2"}
	port := 1234
	stateDir := "/not/existent/directory"
	ctx := context.Background()

	hub.SetExecCommand(exectest.NewCommand(gpupgrade_agent))
	defer hub.ResetExecCommand()

	t.Run("does not start running agents", func(t *testing.T) {
		dialer := func(ctx context.Context, address string) (net.Conn, error) {
			return listener.Dial()
		}

		restartedHosts, err := hub.RestartAgents(ctx, dialer, hostnames, port, stateDir)
		if err != nil {
			t.Errorf("returned %#v", err)
		}
		if len(restartedHosts) != 0 {
			t.Errorf("restarted hosts %v", restartedHosts)
		}
	})

	t.Run("only restarts down agents", func(t *testing.T) {
		expectedHost := "host1"

		dialer := func(ctx context.Context, address string) (net.Conn, error) {
			if strings.HasPrefix(address, expectedHost) { //fail connection attempts to expectedHost
				return nil, immediateFailure{}
			}

			return listener.Dial()
		}

		restartedHosts, err := hub.RestartAgents(ctx, dialer, hostnames, port, stateDir)
		if err != nil {
			t.Errorf("returned %#v", err)
		}

		if len(restartedHosts) != 1 {
			t.Errorf("expected one host to be restarted, got %d", len(restartedHosts))
		}

		if restartedHosts[0] != expectedHost {
			t.Errorf("expected restarted host %s got: %v", expectedHost, restartedHosts)
		}
	})

	t.Run("returns an error when gpupgrade agent fails", func(t *testing.T) {
		hub.SetExecCommand(exectest.NewCommand(gpupgrade_agent_Errors))

		// we fail all connections here so that RestartAgents will run the
		//  (error producing) gpupgrade_agent_Errors
		dialer := func(ctx context.Context, address string) (net.Conn, error) {
			return nil, immediateFailure{}
		}

		restartedHosts, err := hub.RestartAgents(ctx, dialer, hostnames, port, stateDir)
		if err == nil {
			t.Errorf("expected restart agents to fail")
		}

		if merr, ok := err.(*multierror.Error); ok {
			if merr.Len() != 2 {
				t.Errorf("expected 2 errors, got %d", merr.Len())
			}

			var exitErr *exec.ExitError
			for _, err := range merr.WrappedErrors() {
				if !xerrors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Errorf("expected exit code: 1 but got: %#v", err)
				}
			}
		}

		if len(restartedHosts) != 0 {
			t.Errorf("restarted hosts %v", restartedHosts)
		}
	})

	t.Run("starts agents with correct args including specified port and state directory", func(t *testing.T) {
		host := "host1"

		execCmd := exectest.NewCommandWithVerifier(gpupgrade_agent, func(name string, args ...string) {
			if name != "ssh" {
				t.Errorf("RestartAgents invoked with %q want ssh", name)
			}

			cmd := fmt.Sprintf("bash -c \"%s/gpupgrade agent --daemonize --port %d --state-directory %s\"", mustGetExecutablePath(t), port, stateDir)
			expected := []string{host, cmd}
			if !reflect.DeepEqual(args, expected) {
				t.Errorf("got %q want %q", args, expected)
			}
		})
		hub.SetExecCommand(execCmd)
		defer hub.ResetExecCommand()

		dialer := func(ctx context.Context, address string) (net.Conn, error) {
			if strings.HasPrefix(address, host) { // fail connection attempts to host
				return nil, immediateFailure{}
			}

			return listener.Dial()
		}

		_, err := hub.RestartAgents(ctx, dialer, hostnames, port, stateDir)
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}
	})
}

func mustGetExecutablePath(t *testing.T) string {
	t.Helper()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("failed getting test executable path: %#v", err)
	}

	return filepath.Dir(path)
}

// immediateFailure is an error that is explicitly marked non-temporary for
// gRPC's definition of "temporary connection failures". Return this from a
// Dialer implementation to fail fast instead of waiting for the full connection
// timeout.
//
// It seems like gRPC should treat any error that doesn't implement Temporary()
// as non-temporary, but it doesn't; we have to explicitly say that it's _not_
// temporary...
type immediateFailure struct{}

func (_ immediateFailure) Error() string   { return "failing fast" }
func (_ immediateFailure) Temporary() bool { return false }
