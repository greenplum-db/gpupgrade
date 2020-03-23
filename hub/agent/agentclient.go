package agent

import (
	"context"
	"fmt"
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

func (c *Client) Connect(hostnames []string, port int) error {
	if c.connections != nil {
		if err := c.ensureConnectionsAreReady(); err != nil {
			gplog.Error("ensureConnsAreReady failed: %s", err)
			return err
		}

		return nil
	}

	for _, host := range hostnames {
		connection, err := newConnection(host, port, c.grpcDialer)
		if err != nil {
			return err
		}

		c.connections = append(c.connections, connection)
	}

	return nil
}

func (c *Client) Connections() []*Connection {
	return c.connections
}

func (c *Client) CloseConnections() {
	for _, conn := range c.connections {
		defer conn.CancelContext()
		currState := conn.Conn.GetState()
		err := conn.Conn.Close()
		if err != nil {
			gplog.Info(fmt.Sprintf("Error closing hub to agent connection. host: %s, err: %s", conn.Hostname, err.Error()))
		}
		conn.Conn.WaitForStateChange(context.Background(), currState)
	}
}

func (c *Client) StopAllAgents() error {
	var wg sync.WaitGroup
	errs := make(chan error, len(c.connections))

	for _, conn := range c.connections {
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

func (c *Client) RestartAllAgents(ctx context.Context,
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
