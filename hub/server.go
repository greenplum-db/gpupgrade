package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

// Returned from Server.Start() if Server.Stop() has already been called.
var ErrHubStopped = errors.New("hub is stopped")

type Server struct {
	*Config

	StateDir string

	agentClient *agent.Client

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

func New(conf *Config, grpcDialer agent.Dialer, stateDir string) *Server {
	h := &Server{
		Config:      conf,
		StateDir:    stateDir,
		stopped:     make(chan struct{}, 1),
		agentClient: agent.NewClient(grpcDialer),
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
		return errors.Wrap(err, "failed to listen")
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
		err = errors.Wrap(err, "failed to serve")
	}

	// inform Stop() that is it is OK to stop now
	s.stopped <- struct{}{}

	return err
}

func (s *Server) StopServices(ctx context.Context, in *idl.StopServicesRequest) (*idl.StopServicesReply, error) {
	// ensure we have connections to the agents
	_, err := s.AgentConns()

	if err != nil {
		gplog.Debug("failed to stop agents: %#v", err)
	}

	err = s.agentClient.StopAllAgents()

	if err != nil {
		gplog.Debug("failed to stop agents: %#v", err)
	}

	s.Stop(false)
	return &idl.StopServicesReply{}, nil
}

func (s *Server) Stop(closeAgentConns bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// StopServices calls Stop(false) because it has already closed the agentConns
	if closeAgentConns {
		s.agentClient.CloseConnections()
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
	restartedHosts, err := s.agentClient.RestartAllAgents(ctx,
		SegmentHosts(s.Source),
		s.AgentPort,
		s.StateDir)

	return &idl.RestartAgentsReply{AgentHosts: restartedHosts}, err
}

func (s *Server) AgentConns() ([]*agent.Connection, error) {
	// Lock the mutex to protect against races with Server.Stop().
	// XXX This is a *ridiculously* broad lock. Have fun waiting for the dial
	// timeout when calling Stop() and AgentConns() at the same time, for
	// instance. We should not lock around a network operation, but it seems
	// like the AgentConns concept is not long for this world anyway.
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.agentClient.Connect(SegmentHosts(s.Source), s.AgentPort)

	if err != nil {
		return nil, err
	}

	return s.agentClient.Connections(), nil
}

type InitializeConfig struct {
	Standby   greenplum.SegConfig
	Master    greenplum.SegConfig
	Primaries []greenplum.SegConfig
	Mirrors   []greenplum.SegConfig
}

// Config contains all the information that will be persisted to/loaded from
// from disk during calls to Save() and Load().
type Config struct {
	Source *greenplum.Cluster
	Target *greenplum.Cluster

	// TargetInitializeConfig contains all the info needed to initialize the
	// target cluster's master, standby, primaries and mirrors.
	TargetInitializeConfig InitializeConfig

	Port        int
	AgentPort   int
	UseLinkMode bool
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
	// TODO: Switch to an atomic implementation like renameio. Consider what
	// happens if Config.Save() panics: we'll have truncated the file
	// on disk and the hub will be unable to recover. For now, since we normally
	// only save the configuration during initialize and any configuration
	// errors could be fixed by reinitializing, the risk seems small.
	file, err := utils.System.Create(filepath.Join(s.StateDir, ConfigFileName))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			cerr = xerrors.Errorf("closing hub configuration: %w", cerr)
			err = multierror.Append(err, cerr).ErrorOrNil()
		}
	}()

	err = s.Config.Save(file)
	if err != nil {
		return xerrors.Errorf("saving hub configuration: %w", err)
	}

	return nil
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

func SegmentHosts(c *greenplum.Cluster) []string {
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

func MakeTargetClusterMessage(target *greenplum.Cluster) *idl.Message {
	data := make(map[string]string)
	data[idl.ResponseKey_target_port.String()] = strconv.Itoa(target.MasterPort())
	data[idl.ResponseKey_target_master_data_directory.String()] = target.MasterDataDir()

	return &idl.Message{
		Contents: &idl.Message_Response{
			Response: &idl.Response{Data: data},
		},
	}
}
