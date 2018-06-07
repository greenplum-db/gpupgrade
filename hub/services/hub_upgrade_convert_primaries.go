package services

import (
	"fmt"
	"sort"
	"sync"

	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeConvertPrimaries(ctx context.Context, in *pb.UpgradeConvertPrimariesRequest) (*pb.UpgradeConvertPrimariesReply, error) {
	conns, err := h.AgentConns()
	if err != nil {
		gplog.Error("Error connecting to the agents. Err: %v", err)
		return &pb.UpgradeConvertPrimariesReply{}, err
	}
	agentErrs := make(chan error, len(conns))

	dataDirPair, err := h.getDataDirPairs()
	if err != nil {
		gplog.Error("Error getting old and new primary Datadirs. Err: %v", err)
		return &pb.UpgradeConvertPrimariesReply{}, err
	}

	wg := sync.WaitGroup{}
	for _, conn := range conns {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			_, err := pb.NewAgentClient(c.Conn).UpgradeConvertPrimarySegments(context.Background(), &pb.UpgradeConvertPrimarySegmentsRequest{
				OldBinDir:    in.OldBinDir,
				NewBinDir:    in.NewBinDir,
				DataDirPairs: dataDirPair[c.Hostname],
			})

			if err != nil {
				gplog.Error("Hub Upgrade Convert Primaries failed to call agent %s with error: ", c.Hostname, err)
				agentErrs <- err
			}
		}(conn)
	}

	wg.Wait()

	if len(agentErrs) != 0 {
		err = fmt.Errorf("%d agents failed to start pg_upgrade on the primaries. See logs for additional details", len(agentErrs))
	}

	return &pb.UpgradeConvertPrimariesReply{}, err
}

func (h *Hub) getDataDirPairs() (map[string][]*pb.DataDirPair, error) {
	dataDirPairMap := make(map[string][]*pb.DataDirPair)
	oldContents := h.clusterPair.OldCluster.ContentIDs
	newContents := h.clusterPair.NewCluster.ContentIDs
	if len(oldContents) != len(newContents) {
		return nil, fmt.Errorf("Content IDs do not match between old and new clusters")
	}
	sort.Ints(oldContents)
	sort.Ints(newContents)
	for i := range oldContents {
		if oldContents[i] != newContents[i] {
			return nil, fmt.Errorf("Content IDs do not match between old and new clusters")
		}
	}

	for _, contentID := range h.clusterPair.OldCluster.ContentIDs {
		if contentID == -1 {
			continue
		}
		oldSeg := h.clusterPair.OldCluster.Segments[contentID]
		newSeg := h.clusterPair.NewCluster.Segments[contentID]
		if oldSeg.Hostname != newSeg.Hostname {
			return nil, fmt.Errorf("old and new primary segments with content ID %d do not have matching hostnames", contentID)
		}
		dataPair := &pb.DataDirPair{
			OldDataDir: oldSeg.DataDir,
			NewDataDir: newSeg.DataDir,
			OldPort:    int32(oldSeg.Port),
			NewPort:    int32(newSeg.Port),
			Content:    int32(contentID),
		}

		dataDirPairMap[oldSeg.Hostname] = append(dataDirPairMap[oldSeg.Hostname], dataPair)
	}

	return dataDirPairMap, nil
}
