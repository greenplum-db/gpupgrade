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
	"strings"
	"sync"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/reflection"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

var DialTimeout = 3 * time.Second

// Returned from Server.Start() if Server.Stop() has already been called.
var ErrHubStopped = errors.New("hub is stopped")

type Dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type Server struct {
	*Config

	StateDir string

	agentConns []*Connection
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

type Connection struct {
	Conn          *grpc.ClientConn
	AgentClient   idl.AgentClient
	Hostname      string
	CancelContext func()
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
	// FIXME: s.AgentConns() fails fast if a single agent isn't available
	//    we need to connect to all available agents so we can stop just those
	_, err := s.AgentConns()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(s.agentConns))

	for _, conn := range s.agentConns {
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
	restartedHosts, err := RestartAgents(ctx, nil, s.Source.GetHostnames(), s.AgentPort, s.StateDir)
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

func (s *Server) AgentConns() ([]*Connection, error) {
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

	hostnames := s.Source.PrimaryHostnames()
	for _, host := range hostnames {
		ctx, cancelFunc := context.WithTimeout(context.Background(), DialTimeout)
		conn, err := s.grpcDialer(ctx,
			host+":"+strconv.Itoa(s.AgentPort),
			grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			err = errors.Errorf("grpcDialer failed: %s", err.Error())
			gplog.Error(err.Error())
			cancelFunc()
			return nil, err
		}
		s.agentConns = append(s.agentConns, &Connection{
			Conn:          conn,
			AgentClient:   idl.NewAgentClient(conn),
			Hostname:      host,
			CancelContext: cancelFunc,
		})
	}

	return s.agentConns, nil
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

type InitializeConfig struct {
	Standby   utils.SegConfig
	Master    utils.SegConfig
	Primaries []utils.SegConfig
}

// Config contains all the information that will be persisted to/loaded from
// from disk during calls to Save() and Load().
type Config struct {
	Source *utils.Cluster
	Target *utils.Cluster

	// TargetPorts is the list of temporary ports to be used for the target
	// cluster. It's assigned during initial configuration.
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
