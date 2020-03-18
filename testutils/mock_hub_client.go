package testutils

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc"
)

// TODO: we're just about to the point where we can remove this entirely.

type MockHubClient struct {
	ExecuteRequest  *idl.ExecuteRequest
	FinalizeRequest *idl.FinalizeRequest
	Err             error
}

func NewMockHubClient() *MockHubClient {
	return &MockHubClient{}
}

func (m *MockHubClient) Execute(ctx context.Context, in *idl.ExecuteRequest, opts ...grpc.CallOption) (idl.CliToHub_ExecuteClient, error) {
	return nil, nil
}

func (m *MockHubClient) CheckDiskSpace(ctx context.Context, in *idl.CheckDiskSpaceRequest, opts ...grpc.CallOption) (*idl.CheckDiskSpaceReply, error) {
	return nil, nil
}

func (m *MockHubClient) ExecuteInitClusterSubStep() error {
	return nil
}

func (m *MockHubClient) Finalize(ctx context.Context, in *idl.FinalizeRequest, opts ...grpc.CallOption) (idl.CliToHub_FinalizeClient, error) {
	m.FinalizeRequest = in

	return nil, m.Err
}

func (m *MockHubClient) SetConfig(ctx context.Context, in *idl.SetConfigRequest, opts ...grpc.CallOption) (*idl.SetConfigReply, error) {
	return nil, m.Err
}

func (m *MockHubClient) GetConfig(ctx context.Context, in *idl.GetConfigRequest, opts ...grpc.CallOption) (*idl.GetConfigReply, error) {
	return nil, m.Err
}

func (m *MockHubClient) Initialize(ctx context.Context, in *idl.InitializeRequest, opts ...grpc.CallOption) (idl.CliToHub_InitializeClient, error) {
	return nil, nil
}

func (m *MockHubClient) RestartAgents(ctx context.Context, in *idl.RestartAgentsRequest, opts ...grpc.CallOption) (*idl.RestartAgentsReply, error) {
	return nil, nil
}

func (m *MockHubClient) StopServices(ctx context.Context, in *idl.StopServicesRequest, opts ...grpc.CallOption) (*idl.StopServicesReply, error) {
	return nil, nil
}
