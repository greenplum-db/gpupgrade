package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/idl"
)

const dialTimeout = 3 * time.Second

type Connection struct {
	// TODO: make these members package private
	conn          *grpc.ClientConn
	AgentClient   idl.AgentClient
	Hostname      string
	CancelContext func()
}

func (c *Connection) State() connectivity.State {
	return c.conn.GetState()
}

func newConnection(host string, port int, dialer Dialer) (*Connection, error) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), dialTimeout)

	conn, err := dialer(ctx,
		host+":"+strconv.Itoa(port),
		grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		err = errors.Errorf("grpcDialer failed: %s", err.Error())
		gplog.Error(err.Error())
		cancelFunc()
		return nil, err
	}

	return &Connection{
		conn:          conn,
		AgentClient:   idl.NewAgentClient(conn),
		Hostname:      host,
		CancelContext: cancelFunc,
	}, nil
}

func (c *Client) ensureConnectionsAreReady() error {
	notReadyHostnames := []string{}

	for _, conn := range c.connections {
		if conn.conn.GetState() != connectivity.Ready {
			notReadyHostnames = append(notReadyHostnames, conn.Hostname)
		}
	}

	if len(notReadyHostnames) > 0 {
		return fmt.Errorf("the connections to the following hosts were not ready: %s", strings.Join(notReadyHostnames, ","))
	}

	return nil
}
