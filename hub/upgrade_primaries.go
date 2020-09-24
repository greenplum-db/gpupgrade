// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
)

type UpgradePrimaryArgs struct {
	CheckOnly              bool
	MasterBackupDir        string
	AgentConns             []*Connection
	DataDirPairMap         map[string][]*idl.DataDirPair
	Source                 *greenplum.Cluster
	Target                 *greenplum.Cluster
	UseLinkMode            bool
	TablespacesMappingFile string
}

func UpgradePrimaries(args UpgradePrimaryArgs) error {
	request := func(conn *Connection) error {
		_, err := conn.AgentClient.UpgradePrimaries(context.Background(), &idl.UpgradePrimariesRequest{
			SourceBinDir:               filepath.Join(args.Source.GPHome, "bin"),
			TargetBinDir:               filepath.Join(args.Target.GPHome, "bin"),
			TargetVersion:              args.Target.Version.SemVer.String(),
			DataDirPairs:               args.DataDirPairMap[conn.Hostname],
			CheckOnly:                  args.CheckOnly,
			UseLinkMode:                args.UseLinkMode,
			MasterBackupDir:            args.MasterBackupDir,
			TablespacesMappingFilePath: args.TablespacesMappingFile,
		})
		if err != nil {
			failedAction := "upgrade"
			if args.CheckOnly {
				failedAction = "check"
			}
			return xerrors.Errorf("%s primary segment on host %s: %w", failedAction, conn.Hostname, err)
		}

		return nil
	}

	return ExecuteRPC(args.AgentConns, request)
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
			Tablespaces:   getProtoTablespaceMap(s.Tablespaces, targetSeg.DbID),
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

func getProtoTablespaceMap(tablespaces greenplum.Tablespaces, dbId int) map[int32]*idl.TablespaceInfo {
	if tablespaces == nil {
		return nil
	}

	segTablespaces := tablespaces[dbId]
	t := make(map[int32]*idl.TablespaceInfo)
	for tablespaceOid, tablespace := range segTablespaces {
		t[int32(tablespaceOid)] = &idl.TablespaceInfo{
			Location:    tablespace.Location,
			UserDefined: tablespace.IsUserDefined()}
	}

	return t
}
