// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const executeMasterBackupName = "upgraded-master.bak"

func (s *Server) Execute(req *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	upgradedMasterBackupDir := filepath.Join(s.StateDir, executeMasterBackupName)

	st, err := step.Begin(idl.Step_EXECUTE, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("execute: %s", err))
		}
	}()

	st.Run(idl.Substep_SHUTDOWN_SOURCE_CLUSTER, func(streams step.OutStreams) error {
		return s.Source.Stop(streams)
	})

	st.Run(idl.Substep_UPGRADE_MASTER, func(streams step.OutStreams) error {
		stateDir := s.StateDir
		return UpgradeMaster(UpgradeMasterArgs{
			Source:       s.Source,
			Intermediate: s.Intermediate,
			StateDir:     stateDir,
			Stream:       streams,
			CheckOnly:    false,
			UseLinkMode:  s.UseLinkMode,
		})
	})

	st.Run(idl.Substep_COPY_MASTER, func(streams step.OutStreams) error {
		err := CopyMasterDataDir(streams, s.Intermediate.MasterDataDir(), upgradedMasterBackupDir, s.Intermediate.PrimaryHostnames())
		if err != nil {
			return err
		}

		return CopyMasterTablespaces(streams, s.TablespacesMappingFilePath, s.Tablespaces, utils.GetTablespaceDir(), s.Intermediate.PrimaryHostnames())
	})

	st.Run(idl.Substep_UPGRADE_PRIMARIES, func(_ step.OutStreams) error {
		dataDirPair, err := s.GetDataDirPairs()

		if err != nil {
			return xerrors.Errorf("get source and target primary data directories: %w", err)
		}

		return UpgradePrimaries(UpgradePrimaryArgs{
			CheckOnly:              false,
			MasterBackupDir:        upgradedMasterBackupDir,
			AgentConns:             s.agentConns,
			DataDirPairMap:         dataDirPair,
			Source:                 s.Source,
			Intermediate:           s.Intermediate,
			UseLinkMode:            s.UseLinkMode,
			TablespacesMappingFile: s.TablespacesMappingFilePath,
		})
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		return s.Intermediate.Start(streams)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_ExecuteResponse{
		ExecuteResponse: &idl.ExecuteResponse{
			Target: &idl.Cluster{
				Port:                int32(s.Intermediate.MasterPort()),
				MasterDataDirectory: s.Intermediate.MasterDataDir(),
			}},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
