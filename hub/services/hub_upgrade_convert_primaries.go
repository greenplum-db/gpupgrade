package services

import (
	"fmt"
	"sync"

	"github.com/greenplum-db/gpupgrade/hub/configutils"
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
	h.configreader.OfOldClusterConfig(h.conf.StateDir)
	oldConfig := h.configreader.GetSegmentConfiguration()

	h.configreader.OfNewClusterConfig(h.conf.StateDir)
	newConfig := h.configreader.GetSegmentConfiguration()

	oldConfigMap := make(map[int]configutils.Segment)
	for _, segment := range oldConfig {
		if segment.PreferredRole == "p" && segment.Content != -1 {
			oldConfigMap[segment.Content] = segment
		}
	}

	newConfigMap := make(map[int]configutils.Segment)
	for _, segment := range newConfig {
		if segment.PreferredRole == "p" && segment.Content != -1 {
			newConfigMap[segment.Content] = segment
		}
	}

	for contentID, oldSegment := range oldConfigMap {
		newSegment, exists := newConfigMap[contentID]
		if !exists {
			return nil, fmt.Errorf("could not find "+
				"new data directory to match with old data directory for content id %v", contentID)
		}

		hostname := oldSegment.Hostname
		if oldSegment.Hostname != newSegment.Hostname {
			return nil, fmt.Errorf("old and new "+
				"primary segments with content ID %v do not have matching hostnames", contentID)
		}

		dataPair := &pb.DataDirPair{
			OldDataDir: oldSegment.Datadir,
			NewDataDir: newSegment.Datadir,
			OldPort:    int32(oldSegment.Port),
			NewPort:    int32(newSegment.Port),
			Content:    int32(contentID),
		}

		dataDirPairMap[hostname] = append(dataDirPairMap[hostname], dataPair)
	}

	return dataDirPairMap, nil
}
