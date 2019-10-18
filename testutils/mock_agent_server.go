package testutils

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/greenplum-db/gpupgrade/hub/services"

	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc"
)

type MockAgentServer struct {
	addr       net.Addr
	grpcServer *grpc.Server
	numCalls   int
	mu         sync.Mutex

	StatusConversionRequest              *idl.CheckConversionStatusRequest
	StatusConversionResponse             *idl.CheckConversionStatusReply
	UpgradeConvertPrimarySegmentsRequest *idl.UpgradePrimariesRequest
	CreateSegmentDataDirRequest          *idl.CreateSegmentDataDirRequest
	CopyMasterDirRequest                 *idl.CopyMasterDirRequest

	Err chan error
}

// NewMockAgentServer starts a locally running Agent server and returns a struct
// that facilitates unit testing. It also returns a services.Dialer that will
// redirect any outgoing connections to this mock server, as well as the port
// that the server is listening on.
func NewMockAgentServer() (*MockAgentServer, services.Dialer, int) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	mockServer := &MockAgentServer{
		addr:       lis.Addr(),
		grpcServer: grpc.NewServer(),
		Err:        make(chan error, 10000),
	}

	idl.RegisterAgentServer(mockServer.grpcServer, mockServer)

	go func() {
		mockServer.grpcServer.Serve(lis)
	}()

	// Target this running server during dial.
	port := lis.Addr().(*net.TCPAddr).Port
	dialer := func(ctx context.Context, _ string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
		authority := fmt.Sprintf("127.0.0.1:%d", port)
		return grpc.DialContext(ctx, authority, opts...)
	}

	return mockServer, dialer, port
}

func (m *MockAgentServer) CheckUpgradeStatus(context.Context, *idl.CheckUpgradeStatusRequest) (*idl.CheckUpgradeStatusReply, error) {
	m.increaseCalls()

	return &idl.CheckUpgradeStatusReply{}, nil
}

func (m *MockAgentServer) CheckConversionStatus(ctx context.Context, in *idl.CheckConversionStatusRequest) (*idl.CheckConversionStatusReply, error) {
	m.increaseCalls()

	m.StatusConversionRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return m.StatusConversionResponse, err
}

func (m *MockAgentServer) CheckDiskSpaceOnAgents(context.Context, *idl.CheckDiskSpaceRequestToAgent) (*idl.CheckDiskSpaceReplyFromAgent, error) {
	m.increaseCalls()

	return &idl.CheckDiskSpaceReplyFromAgent{}, nil
}

func (m *MockAgentServer) AgentExecuteUpgradePrimariesSubStep(ctx context.Context, in *idl.UpgradePrimariesRequest) (*idl.UpgradePrimariesReply, error) {
	m.increaseCalls()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpgradeConvertPrimarySegmentsRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return &idl.UpgradePrimariesReply{}, err
}

func (m *MockAgentServer) CreateSegmentDataDirectories(ctx context.Context, in *idl.CreateSegmentDataDirRequest) (*idl.CreateSegmentDataDirReply, error) {
	m.increaseCalls()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateSegmentDataDirRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return &idl.CreateSegmentDataDirReply{}, err
}

func (m *MockAgentServer) CopyMasterDirectoryToSegmentDirectories(ctx context.Context, in *idl.CopyMasterDirRequest) (*idl.CopyMasterDirReply, error) {
	m.increaseCalls()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.CopyMasterDirRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return &idl.CopyMasterDirReply{}, err
}

func (m *MockAgentServer) Stop() {
	m.grpcServer.Stop()
}

func (m *MockAgentServer) increaseCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.numCalls++
}

func (m *MockAgentServer) NumberOfCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.numCalls
}
