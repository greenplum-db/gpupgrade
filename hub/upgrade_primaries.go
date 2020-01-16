package hub

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
)

func (h *Hub) ConvertPrimaries(checkOnly bool) error {
	agentConns, err := h.AgentConns()
	if err != nil {
		return errors.Wrap(err, "failed to connect to gpupgrade agent")
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

			_, err := idl.NewAgentClient(conn.Conn).UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{
				OldBinDir:    h.Source.BinDir,
				NewBinDir:    h.Target.BinDir,
				NewVersion:   h.Target.Version.SemVer.String(),
				DataDirPairs: dataDirPair[conn.Hostname],
				CheckOnly:    checkOnly,
			})

			if err != nil {
				agentErrs <- errors.Wrapf(err, "gpupgrade agent failed to convert primary segment on host %s", conn.Hostname)
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

	oldContents := h.Source.ContentIDs
	newContents := h.Target.ContentIDs
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

	for _, contentID := range h.Source.ContentIDs {
		if contentID == -1 {
			continue
		}
		oldSeg := h.Source.Segments[contentID]
		newSeg := h.Target.Segments[contentID]
		if oldSeg.Hostname != newSeg.Hostname {
			return nil, fmt.Errorf("hostnames do not match between old and new cluster with content ID %d. Found old cluster hostname: '%s', and new cluster hostname: '%s'", contentID, oldSeg.Hostname, newSeg.Hostname)
		}

		dataPair := &idl.DataDirPair{
			OldDataDir: oldSeg.DataDir,
			NewDataDir: newSeg.DataDir,
			OldPort:    int32(oldSeg.Port),
			NewPort:    int32(newSeg.Port),
			Content:    int32(contentID),
			DBID:       int32(oldSeg.DbID),
		}

		dataDirPairMap[oldSeg.Hostname] = append(dataDirPairMap[oldSeg.Hostname], dataPair)
	}

	return dataDirPairMap, nil
}
