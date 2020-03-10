package hub

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

type UpgradePrimaryArgs struct {
	CheckOnly       bool
	MasterBackupDir string
	AgentConns      []*Connection
	DataDirPairMap  map[string][]*idl.DataDirPair
	Source          *utils.Cluster
	Target          *utils.Cluster
	UseLinkMode     bool
}

func UpgradePrimaries(args UpgradePrimaryArgs) error {
	wg := sync.WaitGroup{}

	agentErrs := make(chan error, len(args.AgentConns))
	for _, conn := range args.AgentConns {
		wg.Add(1)

		go func(conn *Connection) {
			defer wg.Done()

			_, err := conn.AgentClient.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{
				SourceBinDir:    args.Source.BinDir,
				TargetBinDir:    args.Target.BinDir,
				TargetVersion:   args.Target.Version.SemVer.String(),
				DataDirPairs:    args.DataDirPairMap[conn.Hostname],
				CheckOnly:       args.CheckOnly,
				UseLinkMode:     args.UseLinkMode,
				MasterBackupDir: args.MasterBackupDir,
			})

			if err != nil {
				agentErrs <- errors.Wrapf(err, "failed to upgrade primary segment on host %s", conn.Hostname)
			}
		}(conn)
	}

	wg.Wait()
	close(agentErrs)

	var err error

	for agentErr := range agentErrs {
		err = multierror.Append(err, agentErr)
	}

	return err
}

// ErrInvalidCluster is returned by GetDataDirPairs if the source and target
// clusters content id's clusters do not match.
var ErrInvalidCluster = errors.New("Source and target clusters do not match")

func (s *Server) GetDataDirPairs() (map[string][]*idl.DataDirPair, error) {
	dataDirPairMap := make(map[string][]*idl.DataDirPair)

	sourceContents := s.Source.ContentIDs
	targetContents := s.Target.ContentIDs
	if len(sourceContents) != len(targetContents) {
		return nil, newInvalidClusterError("Source cluster has %d segments, and target cluster has %d segments.", len(sourceContents), len(targetContents))
	}
	sort.Ints(sourceContents)
	sort.Ints(targetContents)
	for i := range sourceContents {
		if sourceContents[i] != targetContents[i] {
			return nil, newInvalidClusterError("Source cluster with content %d, does not match target cluster with content %d.", sourceContents[i], targetContents[i])
		}
	}

	for _, contentID := range s.Source.ContentIDs {
		if contentID == -1 {
			continue
		}
		sourceSeg := s.Source.Primaries[contentID]
		targetSeg := s.Target.Primaries[contentID]
		if sourceSeg.Hostname != targetSeg.Hostname {
			return nil, newInvalidClusterError(
				"hostnames do not match between source and target cluster with content ID %d. "+
					"Found source cluster hostname: '%s', and target cluster hostname: '%s'",
				contentID, sourceSeg.Hostname, targetSeg.Hostname)
		}

		dataPair := &idl.DataDirPair{
			SourceDataDir: sourceSeg.DataDir,
			TargetDataDir: targetSeg.DataDir,
			SourcePort:    int32(sourceSeg.Port),
			TargetPort:    int32(targetSeg.Port),
			Content:       int32(contentID),
			DBID:          int32(sourceSeg.DbID),
		}

		dataDirPairMap[sourceSeg.Hostname] = append(dataDirPairMap[sourceSeg.Hostname], dataPair)
	}

	return dataDirPairMap, nil
}

// InvalidClusterError is the backing error type for ErrInvalidCluster. It
// contains the offending configuration object.
type InvalidClusterError struct {
	msg string
}

func newInvalidClusterError(format string, a ...interface{}) *InvalidClusterError {
	return &InvalidClusterError{
		msg: fmt.Sprintf(format, a...),
	}
}

func (i *InvalidClusterError) Error() string {
	return fmt.Sprintf("Source and target clusters do not match: %s", i.msg)
}

func (i *InvalidClusterError) Is(err error) bool {
	return err == ErrInvalidCluster
}
