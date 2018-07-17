package testutils

import (
	"context"
	"net"
	"sync"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc"
)

type MockAgentServer struct {
	addr       net.Addr
	grpcServer *grpc.Server
	numCalls   int
	mu         sync.Mutex

	StatusConversionRequest              *pb.CheckConversionStatusRequest
	StatusConversionResponse             *pb.CheckConversionStatusReply
	UpgradeConvertPrimarySegmentsRequest *pb.UpgradeConvertPrimarySegmentsRequest
	CreateSegmentDataDirRequest          *pb.CreateSegmentDataDirRequest

	Err chan error
}

func NewMockAgentServer() (*MockAgentServer, int) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	mockServer := &MockAgentServer{
		addr:       lis.Addr(),
		grpcServer: grpc.NewServer(),
		Err:        make(chan error, 10000),
	}

	pb.RegisterAgentServer(mockServer.grpcServer, mockServer)

	go func() {
		mockServer.grpcServer.Serve(lis)
	}()

	return mockServer, lis.Addr().(*net.TCPAddr).Port
}

func (m *MockAgentServer) CheckUpgradeStatus(context.Context, *pb.CheckUpgradeStatusRequest) (*pb.CheckUpgradeStatusReply, error) {
	m.increaseCalls()

	return &pb.CheckUpgradeStatusReply{}, nil
}

func (m *MockAgentServer) CheckConversionStatus(ctx context.Context, in *pb.CheckConversionStatusRequest) (*pb.CheckConversionStatusReply, error) {
	m.increaseCalls()

	m.StatusConversionRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return m.StatusConversionResponse, err
}

func (m *MockAgentServer) CheckDiskSpaceOnAgents(context.Context, *pb.CheckDiskSpaceRequestToAgent) (*pb.CheckDiskSpaceReplyFromAgent, error) {
	m.increaseCalls()

	return &pb.CheckDiskSpaceReplyFromAgent{}, nil
}

func (m *MockAgentServer) PingAgents(context.Context, *pb.PingAgentsRequest) (*pb.PingAgentsReply, error) {
	m.increaseCalls()

	return &pb.PingAgentsReply{}, nil
}

func (m *MockAgentServer) UpgradeConvertPrimarySegments(ctx context.Context, in *pb.UpgradeConvertPrimarySegmentsRequest) (*pb.UpgradeConvertPrimarySegmentsReply, error) {
	m.increaseCalls()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpgradeConvertPrimarySegmentsRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return &pb.UpgradeConvertPrimarySegmentsReply{}, err
}

func (m *MockAgentServer) CreateSegmentDataDirectories(ctx context.Context, in *pb.CreateSegmentDataDirRequest) (*pb.CreateSegmentDataDirReply, error) {
	m.increaseCalls()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateSegmentDataDirRequest = in

	var err error
	if len(m.Err) != 0 {
		err = <-m.Err
	}

	return &pb.CreateSegmentDataDirReply{}, err
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
