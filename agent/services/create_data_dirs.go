package services

import (
	"context"
	"os"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) CreateSegmentDataDirectories(ctx context.Context, in *pb.CreateSegmentDataDirRequest) (*pb.CreateSegmentDataDirReply, error) {
	gplog.Info("got a request to create segment data directories from the hub")

	datadirs := in.Datadirs
	for _, segDataDir := range datadirs {
		_, err := utils.System.Stat(segDataDir)
		if os.IsNotExist(err) {
			err = os.Mkdir(segDataDir, 0755)
			if err != nil {
				gplog.Error("Error creating directory %s: %s", segDataDir, err)
				return &pb.CreateSegmentDataDirReply{}, err
			}
			gplog.Info("Successfully created directory %s", segDataDir)
		} else if err != nil {
			gplog.Error("Error statting new directory %s: %s", segDataDir, err)
			return &pb.CreateSegmentDataDirReply{}, err
		} else {
			gplog.Info("Directory %s already exists", segDataDir)
		}
	}
	return &pb.CreateSegmentDataDirReply{}, nil
}
