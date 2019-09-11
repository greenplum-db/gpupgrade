package commanders_test

import (
	"github.com/greenplum-db/gpupgrade/idl"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type spyCliToHubClient struct {
	idl.CliToHubClient

	checkSeginstallCount int

	statusUpgradeCount int
	statusUpgradeReply *idl.StatusUpgradeReply

	statusConversionCount int
	statusConversionReply *idl.StatusConversionReply

	err error
}

func newSpyCliToHubClient() *spyCliToHubClient {
	return &spyCliToHubClient{
		statusUpgradeReply: &idl.StatusUpgradeReply{},
	}
}

func (s *spyCliToHubClient) StatusUpgrade(
	ctx context.Context,
	request *idl.StatusUpgradeRequest,
	opts ...grpc.CallOption,
) (*idl.StatusUpgradeReply, error) {

	s.statusUpgradeCount++
	return s.statusUpgradeReply, s.err
}

func (s *spyCliToHubClient) StatusConversion(
	ctx context.Context,
	request *idl.StatusConversionRequest,
	opts ...grpc.CallOption,
) (*idl.StatusConversionReply, error) {

	s.statusConversionCount++
	return s.statusConversionReply, s.err
}
