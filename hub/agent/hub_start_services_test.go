package agent_test

import (
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/greenplum-db/gpupgrade/agent"
	hubAgent "github.com/greenplum-db/gpupgrade/hub/agent"
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
	port := 6416
	stateDir := "/not/existent/directory"
	ctx := context.Background()

	hubAgent.SetExecCommand(exectest.NewCommand(gpupgrade_agent))
	defer hubAgent.ResetExecCommand()

	TestDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return nil, nil
	}

	t.Run("returns an error when gpupgrade agent fails", func(t *testing.T) {
		hubAgent.SetExecCommand(exectest.NewCommand(gpupgrade_agent_Errors))

		client := hubAgent.NewClient(TestDialer)

		restartedHosts, err := client.RestartAllAgents(ctx, hostnames, port, stateDir)
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
