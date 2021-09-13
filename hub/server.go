// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/reflection"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

var DialTimeout = 3 * time.Second

// Returned from Server.Start() if Server.Stop() has already been called.
var ErrHubStopped = errors.New("hub is stopped")

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type Server struct {
	*Config

	StateDir string

	agentConns []*idl.Connection
	grpcDialer Dialer

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

func New(conf *Config, grpcDialer Dialer, stateDir string) *Server {
	h := &Server{
		Config:     conf,
		StateDir:   stateDir,
		stopped:    make(chan struct{}, 1),
		grpcDialer: grpcDialer,
	}

	return h
}

// MakeDaemon tells the Server to disconnect its stdout/stderr streams after
// successfully starting up.
func (s *Server) MakeDaemon() {
	s.daemon = true
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(s.Port))
	if err != nil {
		return xerrors.Errorf("listen on port %d: %w", s.Port, err)
	}

	// Set up an interceptor function to log any panics we get from request
	// handlers.
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer log.WritePanics()
		return handler(ctx, req)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

	s.mu.Lock()
	if s.stopped == nil {
		// Stop() has already been called; return without serving.
		s.mu.Unlock()
		return ErrHubStopped
	}
	s.server = server
	s.lis = lis
	s.mu.Unlock()

	idl.RegisterCliToHubServer(server, s)
	reflection.Register(server)

	if s.daemon {
		fmt.Printf("Hub started on port %d (pid %d)\n", s.Port, os.Getpid())
		daemon.Daemonize()
	}

	err = server.Serve(lis)
	if err != nil {
		err = xerrors.Errorf("serve: %w", err)
	}

	// inform Stop() that is it is OK to stop now
	s.stopped <- struct{}{}

	return err
}

func (s *Server) StopServices(ctx context.Context, in *idl.StopServicesRequest) (*idl.StopServicesReply, error) {
	err := s.StopAgents()
	if err != nil {
		gplog.Debug("failed to stop agents: %#v", err)
	}

	s.Stop(false)
	return &idl.StopServicesReply{}, nil
}

// TODO: add unit tests for this; this is currently tricky due to h.AgentConns()
//    mutating global state
func (s *Server) StopAgents() error {
	request := func(conn *idl.Connection) error {
		_, err := conn.AgentClient.StopAgent(context.Background(), &idl.StopAgentRequest{})
		if err == nil { // no error means the agent did not terminate as expected
			return xerrors.Errorf("failed to stop agent on host: %s", conn.Hostname)
		}

		// XXX: "transport is closing" is not documented but is needed to uniquely interpret codes.Unavailable
		// https://github.com/grpc/grpc/blob/v1.24.0/doc/statuscodes.md
		errStatus := grpcStatus.Convert(err)
		if errStatus.Code() != codes.Unavailable || errStatus.Message() != "transport is closing" {
			return xerrors.Errorf("failed to stop agent on host %s : %w", conn.Hostname, err)
		}

		return nil
	}

	// FIXME: s.AgentConns() fails fast if a single agent isn't available
	//    we need to connect to all available agents so we can stop just those
	_, err := s.AgentConns()
	if err != nil {
		return err
	}
	return ExecuteRPC(s.agentConns, request)
}

func (s *Server) Stop(closeAgentConns bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// StopServices calls Stop(false) because it has already closed the agentConns
	if closeAgentConns {
		s.closeAgentConns()
	}

	if s.server != nil {
		s.server.Stop()
		<-s.stopped // block until it is OK to stop
	}

	// Mark this server stopped so that a concurrent Start() doesn't try to
	// start things up again.
	s.stopped = nil
}

func (s *Server) RestartAgents(ctx context.Context, in *idl.RestartAgentsRequest) (*idl.RestartAgentsReply, error) {
	restartedHosts, err := RestartAgents(ctx, nil, AgentHosts(s.Source), s.AgentPort, s.StateDir)
	if err != nil {
		return &idl.RestartAgentsReply{}, err
	}

	_, err = s.AgentConns()
	if err != nil {
		return &idl.RestartAgentsReply{}, xerrors.Errorf("ensuring agent connections are ready: %w", err)
	}

	return &idl.RestartAgentsReply{AgentHosts: restartedHosts}, err
}

func RestartAgents(ctx context.Context,
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

			path, err := utils.GetGpupgradePath()
			if err != nil {
				errs <- err
				return
			}
			cmd := execCommand("ssh", host,
				fmt.Sprintf("bash -c \"%s agent --daemonize --port %d --state-directory %s\"", path, port, stateDir))
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

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return hosts, err
}

func (s *Server) AgentConns() ([]*idl.Connection, error) {
	// Lock the mutex to protect against races with Server.Stop().
	// XXX This is a *ridiculously* broad lock. Have fun waiting for the dial
	// timeout when calling Stop() and AgentConns() at the same time, for
	// instance. We should not lock around a network operation, but it seems
	// like the AgentConns concept is not long for this world anyway.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.agentConns != nil {
		err := EnsureConnsAreReady(s.agentConns)
		if err != nil {
			gplog.Error("ensureConnsAreReady failed: %s", err)
			return nil, err
		}

		return s.agentConns, nil
	}

	hostnames := AgentHosts(s.Source)
	for _, host := range hostnames {
		ctx, cancelFunc := context.WithTimeout(context.Background(), DialTimeout)
		conn, err := s.grpcDialer(ctx,
			host+":"+strconv.Itoa(s.AgentPort),
			grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			err = xerrors.Errorf("grpcDialer failed: %w", err)
			gplog.Error(err.Error())
			cancelFunc()
			return nil, err
		}
		s.agentConns = append(s.agentConns, &idl.Connection{
			Conn:          conn,
			AgentClient:   idl.NewAgentClient(conn),
			Hostname:      host,
			CancelContext: cancelFunc,
		})
	}

	return s.agentConns, nil
}

func EnsureConnsAreReady(agentConns []*idl.Connection) error {
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

// Closes all h.agentConns. Callers must hold the Server's mutex.
// TODO: this function assumes that all h.agentConns are _not_ in a terminal
//   state(e.g. already closed).  If so, conn.Conn.WaitForStateChange() can block
//   indefinitely.
func (s *Server) closeAgentConns() {
	for _, conn := range s.agentConns {
		defer conn.CancelContext()
		currState := conn.Conn.GetState()
		err := conn.Conn.Close()
		if err != nil {
			gplog.Info(fmt.Sprintf("Error closing hub to agent connection. host: %s, err: %s", conn.Hostname, err.Error()))
		}
		conn.Conn.WaitForStateChange(context.Background(), currState)
	}
}

// Config contains all the information that will be persisted to/loaded from
// from disk during calls to Save() and Load().
type Config struct {
	LogArchiveDir string

	// Source is the GPDB cluster that is being upgraded. It is populated during
	// the generation of the cluster config in the initialize step; before that,
	// it is nil.
	Source *greenplum.Cluster

	// IntermediateTarget represents the initialized target cluster that is
	// upgraded based on the source and later renamed to match the source
	// cluster.
	IntermediateTarget *greenplum.Cluster

	// Target is the upgraded GPDB cluster. It is populated during the target
	// gpinitsystem execution in the initialize step; before that, it is nil.
	Target *greenplum.Cluster

	// Connection is a utility object that generates connection URIs to the
	// source or target databases.  It also contains the Source.Version and
	// Target.Version internally.
	Connection *greenplum.Conn

	Port            int
	AgentPort       int
	UseLinkMode     bool
	UseHbaHostnames bool
	TargetGPHome    string
	UpgradeID       upgrade.ID

	// Tablespaces contains the tablespace in the database keyed by
	// dbid and tablespace oid
	Tablespaces                greenplum.Tablespaces
	TablespacesMappingFilePath string
	TargetCatalogVersion       string
}

func (c *Config) Load(r io.Reader) error {
	dec := json.NewDecoder(r)
	return dec.Decode(c)
}

func (c *Config) Save(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

// SaveConfig persists the hub's configuration to disk.
func (s *Server) SaveConfig() (err error) {
	var buffer bytes.Buffer
	if err = s.Config.Save(&buffer); err != nil {
		return xerrors.Errorf("save config: %w", err)
	}

	return utils.AtomicallyWrite(upgrade.GetConfigFile(), buffer.Bytes())
}

func (s *Server) GetLogArchiveDir() (string, error) {
	if s.LogArchiveDir != "" {
		return s.LogArchiveDir, nil
	}

	logDir, err := utils.GetLogDir()
	if err != nil {
		return "", err
	}

	s.LogArchiveDir = filepath.Join(filepath.Dir(logDir), upgrade.GetArchiveDirectoryName(s.UpgradeID, time.Now()))
	err = s.SaveConfig()
	if err != nil {
		return "", err
	}

	return s.LogArchiveDir, nil
}

func LoadConfig(conf *Config, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("opening configuration file: %w", err)
	}
	defer file.Close()

	err = conf.Load(file)
	if err != nil {
		return xerrors.Errorf("reading configuration file: %w", err)
	}

	return nil
}

func AgentHosts(c *greenplum.Cluster) []string {
	uniqueHosts := make(map[string]bool)

	excludingMaster := func(seg *greenplum.SegConfig) bool {
		return !seg.IsMaster()
	}

	for _, seg := range c.SelectSegments(excludingMaster) {
		uniqueHosts[seg.Hostname] = true
	}

	hosts := make([]string, 0)
	for host := range uniqueHosts {
		hosts = append(hosts, host)
	}
	return hosts
}
