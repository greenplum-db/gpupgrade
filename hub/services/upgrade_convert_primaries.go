package services

import (
	"fmt"
	"sort"
	"sync"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeConvertPrimaries(ctx context.Context, in *idl.UpgradeConvertPrimariesRequest) (*idl.UpgradeConvertPrimariesReply, error) {
	gplog.Info("starting %s", upgradestatus.CONVERT_PRIMARIES)

	if err := h.convertPrimaries(); err != nil {
		gplog.Error("failed to %s due to %v", upgradestatus.CONVERT_PRIMARIES, err)
		return &idl.UpgradeConvertPrimariesReply{}, err
	}

	return &idl.UpgradeConvertPrimariesReply{}, nil
}

func (h *Hub) convertPrimaries() error {
	agentConns, err := h.AgentConns()
	if err != nil {
		return errors.Wrap(err, "failed to connect to gpupgrade_agent")
	}

	dataDirPair, err := h.getDataDirPairs()
	if err != nil {
		return errors.Wrap(err, "failed to get old and new primary data directories")
	}

	wg := sync.WaitGroup{}
	agentErrs := make(chan error, len(agentConns))
	for _, agentConn := range agentConns {
		wg.Add(1)

		go func(conn *Connection) {
			defer wg.Done()

			_, err := idl.NewAgentClient(conn.Conn).UpgradeConvertPrimarySegments(context.Background(), &idl.UpgradeConvertPrimarySegmentsRequest{
				OldBinDir:    h.source.BinDir,
				NewBinDir:    h.target.BinDir,
				NewVersion:   h.target.Version.SemVer.String(),
				DataDirPairs: dataDirPair[conn.Hostname],
			})

			if err != nil {
				agentErrs <- errors.Wrapf(err, "gpupgrade_agent failed to convert primary segment on host %s", conn.Hostname)
			}
		}(agentConn)
	}

	wg.Wait()
	close(agentErrs)

	for agentErr := range agentErrs {
		err = multierror.Append(err, agentErr)
	}

	return err
}

func (h *Hub) getDataDirPairs() (map[string][]*idl.DataDirPair, error) {
	dataDirPairMap := make(map[string][]*idl.DataDirPair)

	oldContents := h.source.ContentIDs
	newContents := h.target.ContentIDs
	if len(oldContents) != len(newContents) {
		return nil, fmt.Errorf("old and new cluster content identifiers do not match")
	}
	sort.Ints(oldContents)
	sort.Ints(newContents)
	for i := range oldContents {
		if oldContents[i] != newContents[i] {
			return nil, fmt.Errorf("old and new cluster content identifiers do not match")
		}
	}

	for _, contentID := range h.source.ContentIDs {
		if contentID == -1 {
			continue
		}
		oldSeg := h.source.Segments[contentID]
		newSeg := h.target.Segments[contentID]
		if oldSeg.Hostname != newSeg.Hostname {
			return nil, fmt.Errorf("Hostnames do not match between old and new cluster with content ID %d. Found old cluster hostname: '%s', and new cluster hostname: '%s'.", contentID, oldSeg.Hostname, newSeg.Hostname)
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
