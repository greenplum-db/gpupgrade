package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/reflection"
)

var DialTimeout = 3 * time.Second

// Returned from Hub.Start() if Hub.Stop() has already been called.
var ErrHubStopped = errors.New("hub is stopped")

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type Hub struct {
	conf *HubConfig

	agentConns []*Connection
	source     *utils.Cluster
	target     *utils.Cluster
	grpcDialer Dialer
	checklist  upgradestatus.Checklist

	mu     sync.Mutex
	server *grpc.Server
	lis    net.Listener

	// This is used both as a channel to communicate from Start() to
	// Stop() to indicate to Stop() that it can finally terminate
	// and also as a flag to communicate from Stop() to Start() that
	// Stop() had already beed called, so no need to do anything further
	// in Start().
	// Note that when used as a flag, nil value means that Stop() has
	// been called.

	stopped chan struct{}
	daemon  bool
}

type Connection struct {
	Conn          *grpc.ClientConn
	AgentClient   idl.AgentClient
	Hostname      string
	CancelContext func()
}

type HubConfig struct {
	CliToHubPort   int
	HubToAgentPort int
	StateDir       string
	LogDir         string
}

func NewHub(sourceCluster *utils.Cluster, targetCluster *utils.Cluster, grpcDialer Dialer, conf *HubConfig, checklist upgradestatus.Checklist) *Hub {
	h := &Hub{
		stopped:    make(chan struct{}, 1),
		conf:       conf,
		source:     sourceCluster,
		target:     targetCluster,
		grpcDialer: grpcDialer,
		checklist:  checklist,
	}

	return h
}

// MakeDaemon tells the Hub to disconnect its stdout/stderr streams after
// successfully starting up.
func (h *Hub) MakeDaemon() {
	h.daemon = true
}

func (h *Hub) Start() error {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(h.conf.CliToHubPort))
	if err != nil {
		return errors.Wrap(err, "failed to listen")
	}

	// Set up an interceptor function to log any panics we get from request
	// handlers.
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer log.WritePanics()
		return handler(ctx, req)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

	h.mu.Lock()
	if h.stopped == nil {
		// Stop() has already been called; return without serving.
		h.mu.Unlock()
		return ErrHubStopped
	}
	h.server = server
	h.lis = lis
	h.mu.Unlock()

	idl.RegisterCliToHubServer(server, h)
	reflection.Register(server)

	if h.daemon {
		fmt.Printf("Hub started on port %d (pid %d)\n", h.conf.CliToHubPort, os.Getpid())
		daemon.Daemonize()
	}

	err = server.Serve(lis)
	if err != nil {
		err = errors.Wrap(err, "failed to serve")
	}

	// inform Stop() that is it is OK to stop now
	h.stopped <- struct{}{}

	return err
}

func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.closeConns()

	if h.server != nil {
		h.server.Stop()
		<-h.stopped // block until it is OK to stop
	}

	// Mark this server stopped so that a concurrent Start() doesn't try to
	// start things up again.
	h.stopped = nil
}

func (h *Hub) AgentConns() ([]*Connection, error) {
	// Lock the mutex to protect against races with Hub.Stop().
	// XXX This is a *ridiculously* broad lock. Have fun waiting for the dial
	// timeout when calling Stop() and AgentConns() at the same time, for
	// instance. We should not lock around a network operation, but it seems
	// like the AgentConns concept is not long for this world anyway.
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.agentConns != nil {
		err := EnsureConnsAreReady(h.agentConns)
		if err != nil {
			gplog.Error("ensureConnsAreReady failed: ", err)
			return nil, err
		}

		return h.agentConns, nil
	}

	hostnames := h.source.PrimaryHostnames()
	for _, host := range hostnames {
		ctx, cancelFunc := context.WithTimeout(context.Background(), DialTimeout)
		conn, err := h.grpcDialer(ctx,
			host+":"+strconv.Itoa(h.conf.HubToAgentPort),
			grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			err = errors.Errorf("grpcDialer failed: %s", err.Error())
			gplog.Error(err.Error())
			cancelFunc()
			return nil, err
		}
		h.agentConns = append(h.agentConns, &Connection{
			Conn:          conn,
			AgentClient:   idl.NewAgentClient(conn),
			Hostname:      host,
			CancelContext: cancelFunc,
		})
	}

	return h.agentConns, nil
}

func EnsureConnsAreReady(agentConns []*Connection) error {
	hostnames := []string{}
	for _, conn := range agentConns {
		if conn.Conn.GetState() != connectivity.Ready {
			hostnames = append(hostnames, conn.Hostname)
		}
	}

	if len(hostnames) > 0 {
		return fmt.Errorf("the connections to the following hosts were not ready: %s", strings.Join(hostnames, ","))
	}

	return nil
}

// Closes all h.agentConns. Callers must hold the Hub's mutex.
func (h *Hub) closeConns() {
	for _, conn := range h.agentConns {
		defer conn.CancelContext()
		currState := conn.Conn.GetState()
		err := conn.Conn.Close()
		if err != nil {
			gplog.Info(fmt.Sprintf("Error closing hub to agent connection. host: %s, err: %s", conn.Hostname, err.Error()))
		}
		conn.Conn.WaitForStateChange(context.Background(), currState)
	}
}
