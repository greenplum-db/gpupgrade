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

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/reflection"
	grpcStatus "google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/daemon"
	"github.com/greenplum-db/gpupgrade/utils/log"
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

func (h *Hub) StopServices(ctx context.Context, in *idl.StopServicesRequest) (*idl.StopServicesReply, error) {
	err := h.StopAgents()
	if err != nil {
		gplog.Debug("failed to stop agents: %#v", err)
	}

	h.Stop(false)
	return &idl.StopServicesReply{}, nil
}

// TODO: add unit tests for this; this is currently tricky due to h.AgentConns()
//    mutating global state
func (h *Hub) StopAgents() error {
	// FIXME: h.AgentConns() fails fast if a single agent isn't available
	//    we need to connect to all available agents so we can stop just those
	_, err := h.AgentConns()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(h.agentConns))

	for _, conn := range h.agentConns {
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

func (h *Hub) Stop(closeAgentConns bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// StopServices calls Stop(false) because it has already closed the agentConns
	if closeAgentConns {
		h.closeAgentConns()
	}

	if h.server != nil {
		h.server.Stop()
		<-h.stopped // block until it is OK to stop
	}

	// Mark this server stopped so that a concurrent Start() doesn't try to
	// start things up again.
	h.stopped = nil
}

func (h *Hub) RestartAgents(ctx context.Context, in *idl.RestartAgentsRequest) (*idl.RestartAgentsReply, error) {
	restartedHosts, err := RestartAgents(ctx, nil, h.source.GetHostnames(), h.conf.HubToAgentPort, h.conf.StateDir)
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
				fmt.Sprintf("bash -c \"%s --daemonize --state-directory %s\"", agentPath, stateDir))
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
			gplog.Error("ensureConnsAreReady failed: %s", err)
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
// TODO: this function assumes that all h.agentConns are _not_ in a terminal
//   state(e.g. already closed).  If so, conn.Conn.WaitForStateChange() can block
//   indefinitely.
func (h *Hub) closeAgentConns() {
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

// streamStepWriter extends the standard StepWriter, which only writes state to
// disk, with functionality that sends status updates across the given stream.
// (In practice this stream will be a gRPC CliToHub_XxxServer interface.)
type streamStepWriter struct {
	upgradestatus.StateWriter
	stream messageSender
}

type messageSender interface {
	Send(*idl.Message) error // matches gRPC streaming Send()
}

func sendStatus(stream messageSender, step idl.UpgradeSteps, status idl.StepStatus) {
	// A stream is not guaranteed to remain connected during execution, so
	// errors are explicitly ignored.
	_ = stream.Send(&idl.Message{
		Contents: &idl.Message_Status{&idl.UpgradeStepStatus{
			Step:   step,
			Status: status,
		}},
	})
}

func (s streamStepWriter) MarkInProgress() error {
	if err := s.StateWriter.MarkInProgress(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.StepStatus_RUNNING)
	return nil
}

func (s streamStepWriter) MarkComplete() error {
	if err := s.StateWriter.MarkComplete(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.StepStatus_COMPLETE)
	return nil
}

func (s streamStepWriter) MarkFailed() error {
	if err := s.StateWriter.MarkFailed(); err != nil {
		return err
	}

	sendStatus(s.stream, s.Code(), idl.StepStatus_FAILED)
	return nil
}

// Extracts common hub logic to reset state directory, mark step as in-progress,
// and control status streaming.
func (h *Hub) InitializeStep(step string, stream messageSender) (upgradestatus.StateWriter, error) {
	stepWriter := streamStepWriter{
		h.checklist.GetStepWriter(step),
		stream,
	}

	err := stepWriter.ResetStateDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to reset state directory")
	}

	err = stepWriter.MarkInProgress()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set %s to %s", step, file.InProgress)
	}

	return stepWriter, nil
}
