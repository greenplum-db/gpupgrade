package services

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type AgentServer struct {
	GetDiskUsage func() (map[string]float64, error)
	executor     cluster.Executor
	conf         AgentConfig

	mu      sync.Mutex
	server  *grpc.Server
	lis     net.Listener
	stopped chan struct{}
	daemon  bool
}

type AgentConfig struct {
	Port     int
	StateDir string
}

func NewAgentServer(executor cluster.Executor, conf AgentConfig) *AgentServer {
	return &AgentServer{
		GetDiskUsage: diskUsage,
		executor:     executor,
		conf:         conf,
		stopped:      make(chan struct{}, 1),
	}
}

// MakeDaemon tells the AgentServer to disconnect its stdout/stderr streams
// after successfully starting up.
func (a *AgentServer) MakeDaemon() {
	a.daemon = true
}

func (a *AgentServer) Start() {
	createIfNotExists(a.conf.StateDir)
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(a.conf.Port))
	if err != nil {
		gplog.Fatal(err, "failed to listen")
	}

	server := grpc.NewServer()
	a.mu.Lock()
	a.server = server
	a.lis = lis
	a.mu.Unlock()

	pb.RegisterAgentServer(server, a)
	reflection.Register(server)

	// TODO: Research daemonize to see what else may need to be
	// done for the child process to safely detach from the parent
	if a.daemon {
		fmt.Printf("Agent started on port %d (pid %d)\n", a.conf.Port, os.Getpid())
		os.Stderr.Close()
		os.Stdout.Close()
	}

	err = server.Serve(lis)
	if err != nil {
		gplog.Fatal(err, "failed to serve", err)
	}

	a.stopped <- struct{}{}
}

func (a *AgentServer) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.server != nil {
		a.server.Stop()
		<-a.stopped
	}
}

func createIfNotExists(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.Mkdir(dir, 0777)
	}
}
