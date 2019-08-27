package testutils

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"google.golang.org/grpc"
)

type MockHubClient struct {
	UpgradeCopyMasterDataDirRequest *idl.UpgradeCopyMasterDataDirRequest
	UpgradeReconfigurePortsRequest  *idl.UpgradeReconfigurePortsRequest

	UpgradeConvertPrimariesRequest  *idl.UpgradeConvertPrimariesRequest
	UpgradeConvertPrimariesResponse *idl.UpgradeConvertPrimariesReply
	Err                             error
}

func NewMockHubClient() *MockHubClient {
	return &MockHubClient{}
}

func (m *MockHubClient) Ping(ctx context.Context, in *idl.PingRequest, opts ...grpc.CallOption) (*idl.PingReply, error) {
	return nil, nil
}

func (m *MockHubClient) StatusUpgrade(ctx context.Context, in *idl.StatusUpgradeRequest, opts ...grpc.CallOption) (*idl.StatusUpgradeReply, error) {
	return nil, nil
}

func (m *MockHubClient) StatusConversion(ctx context.Context, in *idl.StatusConversionRequest, opts ...grpc.CallOption) (*idl.StatusConversionReply, error) {
	return nil, nil
}

func (m *MockHubClient) CheckConfig(ctx context.Context, in *idl.CheckConfigRequest, opts ...grpc.CallOption) (*idl.CheckConfigReply, error) {
	return nil, nil
}

func (m *MockHubClient) CheckSeginstall(ctx context.Context, in *idl.CheckSeginstallRequest, opts ...grpc.CallOption) (*idl.CheckSeginstallReply, error) {
	return nil, nil
}

func (m *MockHubClient) CheckObjectCount(ctx context.Context, in *idl.CheckObjectCountRequest, opts ...grpc.CallOption) (*idl.CheckObjectCountReply, error) {
	return nil, nil
}

func (m *MockHubClient) CheckVersion(ctx context.Context, in *idl.CheckVersionRequest, opts ...grpc.CallOption) (*idl.CheckVersionReply, error) {
	return nil, nil
}

func (m *MockHubClient) CheckDiskSpace(ctx context.Context, in *idl.CheckDiskSpaceRequest, opts ...grpc.CallOption) (*idl.CheckDiskSpaceReply, error) {
	return nil, nil
}

func (m *MockHubClient) PrepareInitCluster(ctx context.Context, in *idl.PrepareInitClusterRequest, opts ...grpc.CallOption) (*idl.PrepareInitClusterReply, error) {
	return nil, nil
}

func (m *MockHubClient) PrepareShutdownClusters(ctx context.Context, in *idl.PrepareShutdownClustersRequest, opts ...grpc.CallOption) (*idl.PrepareShutdownClustersReply, error) {
	return nil, nil
}

func (m *MockHubClient) UpgradeConvertMaster(ctx context.Context, in *idl.UpgradeConvertMasterRequest, opts ...grpc.CallOption) (*idl.UpgradeConvertMasterReply, error) {
	return nil, nil
}

func (m *MockHubClient) PrepareStartAgents(ctx context.Context, in *idl.PrepareStartAgentsRequest, opts ...grpc.CallOption) (*idl.PrepareStartAgentsReply, error) {
	return nil, nil
}

func (m *MockHubClient) UpgradeCopyMasterDataDir(ctx context.Context, in *idl.UpgradeCopyMasterDataDirRequest, opts ...grpc.CallOption) (*idl.UpgradeCopyMasterDataDirReply, error) {
	m.UpgradeCopyMasterDataDirRequest = in

	return &idl.UpgradeCopyMasterDataDirReply{}, m.Err
}

func (m *MockHubClient) UpgradeValidateStartCluster(ctx context.Context, in *idl.UpgradeValidateStartClusterRequest, opts ...grpc.CallOption) (*idl.UpgradeValidateStartClusterReply, error) {
	return nil, nil
}

func (m *MockHubClient) UpgradeConvertPrimaries(ctx context.Context, in *idl.UpgradeConvertPrimariesRequest, opts ...grpc.CallOption) (*idl.UpgradeConvertPrimariesReply, error) {
	m.UpgradeConvertPrimariesRequest = in

	return m.UpgradeConvertPrimariesResponse, m.Err
}

func (m *MockHubClient) UpgradeReconfigurePorts(ctx context.Context, in *idl.UpgradeReconfigurePortsRequest, opts ...grpc.CallOption) (*idl.UpgradeReconfigurePortsReply, error) {
	m.UpgradeReconfigurePortsRequest = in

	return nil, m.Err
}

func (m *MockHubClient) SetConfig(ctx context.Context, in *idl.SetConfigRequest, opts ...grpc.CallOption) (*idl.SetConfigReply, error) {
	return nil, m.Err
}

func (m *MockHubClient) GetConfig(ctx context.Context, in *idl.GetConfigRequest, opts ...grpc.CallOption) (*idl.GetConfigReply, error) {
	return nil, m.Err
}
