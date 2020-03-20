package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/idl"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

type Client struct {
	connections []*Connection
	grpcDialer  Dialer
}

func NewClient(dialer Dialer) *Client {
	return &Client{
		grpcDialer: dialer,
	}
}

func (m *Client) Connect(hostnames []string, port int) error {
	if m.connections != nil {
		if err := m.ensureConnectionsAreReady(); err != nil {
			gplog.Error("ensureConnsAreReady failed: %s", err)
			return err
		}

		return nil
	}

	for _, host := range hostnames {
		connection, err := newConnection(host, port, m.grpcDialer)
		if err != nil {
			return err
		}

		m.connections = append(m.connections, connection)
	}

	return nil
}

func (m *Client) Connections() []*Connection {
	return m.connections
}

func (m *Client) StopAllAgents() error {
	var wg sync.WaitGroup
	errs := make(chan error, len(m.connections))

	for _, conn := range m.connections {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := conn.AgentClient.StopAgent(context.Background(), &idl.StopAgentRequest{})
			if err == nil { // no error means the agent did not terminate as expected
				errs <- xerrors.Errorf("failed to stop agent on host: %s", conn.Hostname)
				return
			}

			// XXX: "transport is closing" is not documented but is needed to uniquely interpret codes.Unavailable
			// https://github.com/grpc/grpc/blob/v1.24.0/doc/statuscodes.md
			errStatus := grpcStatus.Convert(err)
			if errStatus.Code() != codes.Unavailable || errStatus.Message() != "transport is closing" {
				errs <- xerrors.Errorf("failed to stop agent on host %s : %w", conn.Hostname, err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	var multiErr *multierror.Error
	for err := range errs {
		multiErr = multierror.Append(multiErr, err)
	}

	return multiErr.ErrorOrNil()
}

func (m *Client) CloseConnections() {
	for _, conn := range m.connections {
		defer conn.CancelContext()
		currState := conn.Conn.GetState()
		err := conn.Conn.Close()
		if err != nil {
			gplog.Info(fmt.Sprintf("Error closing hub to agent connection. host: %s, err: %s", conn.Hostname, err.Error()))
		}
		conn.Conn.WaitForStateChange(context.Background(), currState)
	}
}

// TODO: make this a method on HubToAgentClient
func RestartAllAgents(ctx context.Context,
	dialer func(context.Context, string) (net.Conn, error),
	hostnames []string,
	port int,
	stateDir string) ([]string, error) {

	var wg sync.WaitGroup
	restartedHosts := make(chan string, len(hostnames))
	errs := make(chan error, len(hostnames))

	for _, host := range hostnames {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()

			address := host + ":" + strconv.Itoa(port)
			timeoutCtx, cancelFunc := context.WithTimeout(ctx, 3*time.Second)
			opts := []grpc.DialOption{
				grpc.WithBlock(),
				grpc.WithInsecure(),
				grpc.FailOnNonTempDialError(true),
			}
			if dialer != nil {
				opts = append(opts, grpc.WithContextDialer(dialer))
			}
			conn, err := grpc.DialContext(timeoutCtx, address, opts...)
			cancelFunc()
			if err == nil {
				err = conn.Close()
				if err != nil {
					gplog.Error("failed to close agent connection to %s: %+v", host, err)
				}
				return
			}

			gplog.Debug("failed to dial agent on %s: %+v", host, err)
			gplog.Info("starting agent on %s", host)

			agentPath, err := getAgentPath()
			if err != nil {
				errs <- err
				return
			}
			cmd := execCommand("ssh", host,
				fmt.Sprintf("bash -c \"%s agent --daemonize --state-directory %s\"", agentPath, stateDir))
			stdout, err := cmd.Output()
			if err != nil {
				errs <- err
				return
			}

			gplog.Debug(string(stdout))
			restartedHosts <- host
		}(host)
	}

	wg.Wait()
	close(errs)
	close(restartedHosts)

	var hosts []string
	for h := range restartedHosts {
		hosts = append(hosts, h)
	}

	var multiErr *multierror.Error
	for err := range errs {
		multiErr = multierror.Append(multiErr, err)
	}

	return hosts, multiErr.ErrorOrNil()
}

func getAgentPath() (string, error) {
	hubPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(hubPath), "gpupgrade"), nil
}
