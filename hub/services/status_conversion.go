package services

import (
	"fmt"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) StatusConversion(ctx context.Context, in *idl.StatusConversionRequest) (*idl.StatusConversionReply, error) {
	agentConnections, err := h.AgentConns()
	if err != nil {
		return &idl.StatusConversionReply{}, err
	}

	primaryStatuses, err := GetConversionStatusFromPrimaries(agentConnections, h.source)
	if err != nil {
		err := fmt.Errorf("Could not get conversion status from primaries. Err: \"%v\"", err)
		gplog.Error(err.Error())
		return &idl.StatusConversionReply{}, err
	}

	return &idl.StatusConversionReply{
		ConversionStatuses: primaryStatuses,
	}, nil
}

// Helper function to make grpc calls to all agents on primaries for their status
// TODO: Check conversion statuses in parallel
func GetConversionStatusFromPrimaries(conns []*Connection, source *utils.Cluster) ([]*idl.PrimaryStatus, error) {
	var statuses []*idl.PrimaryStatus
	for _, conn := range conns {
		// Build a list of segments on the host in which the agent resides on.
		var agentSegments []*idl.SegmentInfo

		segments, err := source.SegmentsOn(conn.Hostname)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't retrieve target cluster segments")
		}

		for _, segment := range segments {
			agentSegments = append(agentSegments, &idl.SegmentInfo{
				Content: int32(segment.ContentID),
				Dbid:    int32(segment.DbID),
				DataDir: segment.DataDir,
			})
		}

		status, err := conn.AgentClient.CheckConversionStatus(context.Background(), &idl.CheckConversionStatusRequest{
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
func PrimaryConversionStatus(hub *Hub) idl.StepStatus {
	// We can't determine the actual status if there's an error, so we log it and return PENDING
	conversionStatus, err := hub.StatusConversion(nil, &idl.StatusConversionRequest{})
	if err != nil {
		gplog.Error("Could not get primary conversion status: %s", err)
		return idl.StepStatus_PENDING
	}

	statuses := conversionStatus.GetConversionStatuses()
	switch {
	case hasAny(statuses, idl.StepStatus_FAILED):
		return idl.StepStatus_FAILED
	case hasAny(statuses, idl.StepStatus_RUNNING):
		return idl.StepStatus_RUNNING
	case hasAll(statuses, idl.StepStatus_COMPLETE):
		return idl.StepStatus_COMPLETE
	default:
		return idl.StepStatus_PENDING
	}
}

func hasAll(statuses []*idl.PrimaryStatus, status idl.StepStatus) bool { // nolint: unparam
	for _, elem := range statuses {
		if elem.Status != status {
			return false
		}
	}

	return true
}

func hasAny(statuses []*idl.PrimaryStatus, status idl.StepStatus) bool {
	for _, elem := range statuses {
		if elem.Status == status {
			return true
		}
	}

	return false
}
