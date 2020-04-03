package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
)

type Server struct {
	conf Config

	mu      sync.Mutex
	server  *grpc.Server
	lis     net.Listener
	stopped chan struct{}
	daemon  bool
}

type Config struct {
	Port     int
	StateDir string
}

func NewServer(conf Config) *Server {
	return &Server{
		conf:    conf,
		stopped: make(chan struct{}, 1),
	}
}

// MakeDaemon tells the Server to disconnect its stdout/stderr streams after
// successfully starting up.
func (s *Server) MakeDaemon() {
	s.daemon = true
}

func (s *Server) Start() {
	createIfNotExists(s.conf.StateDir)
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(s.conf.Port))
	if err != nil {
		gplog.Fatal(err, "failed to listen")
	}

	// Set up an interceptor function to log any panics we get from request
	// handlers.
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer log.WritePanics()
		return handler(ctx, req)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))

	s.mu.Lock()
	s.server = server
	s.lis = lis
	s.mu.Unlock()

	idl.RegisterAgentServer(server, s)
	reflection.Register(server)

	if s.daemon {
		// Send an identifier string back to the hub, and log it locally for
		// easier debugging.
		info := fmt.Sprintf("Agent started on port %d (pid %d)", s.conf.Port, os.Getpid())

		fmt.Println(info)
		daemon.Daemonize()
		gplog.Info(info)
	}

	err = server.Serve(lis)
	if err != nil {
		gplog.Fatal(err, "failed to serve: %s", err)
	}

	s.stopped <- struct{}{}
}

func (s *Server) StopAgent(ctx context.Context, in *idl.StopAgentRequest) (*idl.StopAgentReply, error) {
	s.Stop()
	return &idl.StopAgentReply{}, nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		s.server.Stop()
		<-s.stopped
	}
}

func createIfNotExists(dir string) {
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return
	}

	if err := os.Mkdir(dir, 0777); err != nil {
		gplog.Fatal(err, "failed to create state directory %q", dir)
	}
}
