package services

import (
	"fmt"
	"sort"
	"sync"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/pkg/errors"
)

func (h *Hub) UpgradeConvertPrimaries(ctx context.Context, in *idl.UpgradeConvertPrimariesRequest) (*idl.UpgradeConvertPrimariesReply, error) {
	gplog.Info("starting %s", upgradestatus.CONVERT_PRIMARIES)
	defer log.WritePanics()

	err := h.convertPrimaries()
	if err != nil {
		gplog.Error(err.Error())
		return &idl.UpgradeConvertPrimariesReply{}, err
	}

	return &idl.UpgradeConvertPrimariesReply{}, nil
}

func (h *Hub) convertPrimaries() error {
	conns, err := h.AgentConns()
	if err != nil {
		return errors.Wrap(err, "failed to connect to agents")
	}

	agentErrs := make(chan error, len(conns))

	dataDirPair, err := h.getDataDirPairs()
	if err != nil {
		return errors.Wrap(err,"failed to get old cluster old's and new primary data directories")
	}

	wg := sync.WaitGroup{}
	for _, conn := range conns {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			_, err := idl.NewAgentClient(c.Conn).UpgradeConvertPrimarySegments(context.Background(), &idl.UpgradeConvertPrimarySegmentsRequest{
				OldBinDir:    h.source.BinDir,
				NewBinDir:    h.target.BinDir,
				NewVersion:   h.target.Version.SemVer.String(),
				DataDirPairs: dataDirPair[c.Hostname],
			})

			if err != nil {
				gplog.Error("Hub Upgrade Convert Primaries failed to call agent %s with error: %v", c.Hostname, err)
				agentErrs <- err
			}
		}(conn)
	}

	wg.Wait()

	if len(agentErrs) != 0 {
		err = fmt.Errorf("%d agents failed to start pg_upgrade on the primaries. See logs for additional details", len(agentErrs))
	}

	return nil
}

func (h *Hub) getDataDirPairs() (map[string][]*idl.DataDirPair, error) {
	dataDirPairMap := make(map[string][]*idl.DataDirPair)
	oldContents := h.source.ContentIDs
	newContents := h.target.ContentIDs
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

	for _, contentID := range h.source.ContentIDs {
		if contentID == -1 {
			continue
		}
		oldSeg := h.source.Segments[contentID]
		newSeg := h.target.Segments[contentID]
		if oldSeg.Hostname != newSeg.Hostname {
			return nil, fmt.Errorf("old and new primary segments with content ID %d do not have matching hostnames", contentID)
		}
		dataPair := &idl.DataDirPair{
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
