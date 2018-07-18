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
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/reflection"
)

var DialTimeout = 3 * time.Second

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type Hub struct {
	conf *HubConfig

	agentConns  []*Connection
	clusterPair *utils.ClusterPair
	grpcDialer  Dialer
	checklist   upgradestatus.Checklist

	mu      sync.Mutex
	server  *grpc.Server
	lis     net.Listener
	stopped chan struct{}
	daemon  bool
}

type Connection struct {
	Conn          *grpc.ClientConn
	AgentClient   pb.AgentClient
	Hostname      string
	CancelContext func()
}

type HubConfig struct {
	CliToHubPort   int
	HubToAgentPort int
	StateDir       string
	LogDir         string
}

func NewHub(pair *utils.ClusterPair, grpcDialer Dialer, conf *HubConfig, checklist upgradestatus.Checklist) *Hub {
	h := &Hub{
		stopped:     make(chan struct{}, 1),
		conf:        conf,
		clusterPair: pair,
		grpcDialer:  grpcDialer,
		checklist:   checklist,
	}

	return h
}

// MakeDaemon tells the Hub to disconnect its stdout/stderr streams after
// successfully starting up.
func (h *Hub) MakeDaemon() {
	h.daemon = true
}

func (h *Hub) Start() {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(h.conf.CliToHubPort))
	if err != nil {
		gplog.Fatal(err, "failed to listen")
	}

	server := grpc.NewServer()
	h.mu.Lock()
	h.server = server
	h.lis = lis
	h.mu.Unlock()

	pb.RegisterCliToHubServer(server, h)
	reflection.Register(server)

	if h.daemon {
		fmt.Printf("Hub started on port %d (pid %d)\n", h.conf.CliToHubPort, os.Getpid())
		daemon.Daemonize()
	}

	err = server.Serve(lis)
	if err != nil {
		gplog.Fatal(err, "failed to serve", err)
	}

	h.stopped <- struct{}{}
}

func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.server != nil {
		h.closeConns()
		h.server.Stop()
		<-h.stopped
	}
}

func (h *Hub) AgentConns() ([]*Connection, error) {
	if h.agentConns != nil {
		err := EnsureConnsAreReady(h.agentConns)
		if err != nil {
			gplog.Error("ensureConnsAreReady failed: ", err)
			return nil, err
		}

		return h.agentConns, nil
	}

	hostnames := h.clusterPair.GetHostnames()
	for _, host := range hostnames {
		ctx, cancelFunc := context.WithTimeout(context.Background(), DialTimeout)
		conn, err := h.grpcDialer(ctx, host+":"+strconv.Itoa(h.conf.HubToAgentPort), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			err = errors.New(fmt.Sprintf("grpcDialer failed: %s", err.Error()))
			gplog.Error(err.Error())
			cancelFunc()
			return nil, err
		}
		h.agentConns = append(h.agentConns, &Connection{
			Conn:          conn,
			AgentClient:   pb.NewAgentClient(conn),
			Hostname:      host,
			CancelContext: cancelFunc,
		})
	}

	return h.agentConns, nil
}

// GetConfig returns a copy of the hub's current configuration.
func (h *Hub) GetConfig() HubConfig {
	return *h.conf
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

func (h *Hub) segmentsByHost() map[string][]cluster.SegConfig {
	segmentsByHost := make(map[string][]cluster.SegConfig)
	for _, segment := range h.clusterPair.OldCluster.Segments {
		host := segment.Hostname
		if len(segmentsByHost[host]) == 0 {
			segmentsByHost[host] = []cluster.SegConfig{segment}
		} else {
			segmentsByHost[host] = append(segmentsByHost[host], segment)
		}
	}

	return segmentsByHost
}
