package services

import (
	"fmt"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) StatusConversion(ctx context.Context, in *pb.StatusConversionRequest) (*pb.StatusConversionReply, error) {
	agentConnections, err := h.AgentConns()
	if err != nil {
		return &pb.StatusConversionReply{}, err
	}

	primaryStatuses, err := GetConversionStatusFromPrimaries(agentConnections, h.source)
	if err != nil {
		err := fmt.Errorf("Could not get conversion status from primaries. Err: \"%v\"", err)
		gplog.Error(err.Error())
		return &pb.StatusConversionReply{}, err
	}

	return &pb.StatusConversionReply{
		ConversionStatuses: primaryStatuses,
	}, nil
}

// Helper function to make grpc calls to all agents on primaries for their status
// TODO: Check conversion statuses in parallel
func GetConversionStatusFromPrimaries(conns []*Connection, source *utils.Cluster) ([]*pb.PrimaryStatus, error) {
	var statuses []*pb.PrimaryStatus
	for _, conn := range conns {
		// Build a list of segments on the host in which the agent resides on.
		var agentSegments []*pb.SegmentInfo

		segments, err := source.SegmentsOn(conn.Hostname)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't retrieve target cluster segments")
		}

		for _, segment := range segments {
			agentSegments = append(agentSegments, &pb.SegmentInfo{
				Content: int32(segment.ContentID),
				Dbid:    int32(segment.DbID),
				DataDir: segment.DataDir,
			})
		}

		status, err := conn.AgentClient.CheckConversionStatus(context.Background(), &pb.CheckConversionStatusRequest{
			Segments: agentSegments,
			Hostname: conn.Hostname,
		})
		if err != nil {
			return nil, fmt.Errorf("agent on host %s returned an error when checking conversion status: %s", conn.Hostname, err)
		}

		statuses = append(statuses, status.GetStatuses()...)
	}

	return statuses, nil
}

// PrimaryConversionStatus a function that matches the interface of Step.Status_
// It is used by the state manager to get status for the CONVERT_PRIMARY step.
func PrimaryConversionStatus(hub *Hub) pb.StepStatus {
	// We can't determine the actual status if there's an error, so we log it and return PENDING
	conversionStatus, err := hub.StatusConversion(nil, &pb.StatusConversionRequest{})
	if err != nil {
		gplog.Error("Could not get primary conversion status: %s", err)
		return pb.StepStatus_PENDING
	}

	finalStatus := pb.StepStatus_PENDING
	for _, status := range conversionStatus.GetConversionStatuses() {
		if status.Status == pb.StepStatus_FAILED {
			return pb.StepStatus_FAILED
		} else if status.Status == pb.StepStatus_RUNNING {
			finalStatus = pb.StepStatus_RUNNING
		} else if status.Status == pb.StepStatus_COMPLETE && finalStatus != pb.StepStatus_RUNNING {
			finalStatus = pb.StepStatus_COMPLETE
		}
	}
	return finalStatus
}
