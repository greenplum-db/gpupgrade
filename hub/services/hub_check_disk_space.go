package services

import (
	"fmt"
	"strconv"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	// todo generalize to any host
	diskUsageWarningLimit = 80
)

func (h *Hub) CheckDiskSpace(ctx context.Context,
	in *pb.CheckDiskSpaceRequest) (*pb.CheckDiskSpaceReply, error) {

	gplog.Info("starting CheckDiskSpace")
	var replyMessages []string
	hostnames := h.clusterPair.GetHostnames()
	var clients []ClientAndHostname
	for i := 0; i < len(hostnames); i++ {
		conn, err := grpc.Dial(hostnames[i]+":"+strconv.Itoa(h.conf.HubToAgentPort), grpc.WithInsecure())
		if err == nil {
			clients = append(clients, ClientAndHostname{Client: pb.NewAgentClient(conn), Hostname: hostnames[i]})
			defer conn.Close()
		} else {
			gplog.Error(err.Error())
			replyMessages = append(replyMessages, "ERROR: couldn't get gRPC conn to "+hostnames[i])
		}
	}
	replyMessages = append(replyMessages, GetDiskSpaceFromSegmentHosts(clients)...)

	return &pb.CheckDiskSpaceReply{SegmentFileSysUsage: replyMessages}, nil
}

func GetDiskSpaceFromSegmentHosts(clients []ClientAndHostname) []string {
	replyMessages := []string{}
	for i := 0; i < len(clients); i++ {
		reply, err := clients[i].Client.CheckDiskSpaceOnAgents(context.Background(),
			&pb.CheckDiskSpaceRequestToAgent{})
		if err != nil {
			gplog.Error(err.Error())
			replyMessages = append(replyMessages, "Could not get disk usage from: "+clients[i].Hostname)
			continue
		}
		foundAnyTooFull := false
		for _, line := range reply.ListOfFileSysUsage {
			if line.Usage >= diskUsageWarningLimit {
				replyMessages = append(replyMessages, fmt.Sprintf("diskspace check - %s - WARNING %s %.1f use",
					clients[i].Hostname, line.Filesystem, line.Usage))
				foundAnyTooFull = true
			}
		}
		if !foundAnyTooFull {
			replyMessages = append(replyMessages, fmt.Sprintf("diskspace check - %s - OK", clients[i].Hostname))
		}
	}

	return replyMessages
}
