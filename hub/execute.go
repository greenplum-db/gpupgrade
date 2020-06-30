// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

const executeMasterBackupName = "upgraded-master.bak"

func (s *Server) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	upgradedMasterBackupDir := filepath.Join(s.StateDir, executeMasterBackupName)

	st, err := step.Begin(s.StateDir, idl.Step_EXECUTE, stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("execute: %s", err))
		}
	}()

	st.Run(idl.Substep_SHUTDOWN_SOURCE_CLUSTER, func(streams step.OutStreams) error {
		err := s.Source.Stop(streams)

		if err != nil {
			return xerrors.Errorf("failed to stop source cluster: %w", err)
		}

		return nil
	})

	st.Run(idl.Substep_UPGRADE_MASTER, func(streams step.OutStreams) error {
		stateDir := s.StateDir
		return UpgradeMaster(UpgradeMasterArgs{
			Source:      s.Source,
			Target:      s.Target,
			StateDir:    stateDir,
			Stream:      streams,
			CheckOnly:   false,
			UseLinkMode: s.UseLinkMode,
		})
	})

	st.Run(idl.Substep_COPY_MASTER, func(streams step.OutStreams) error {
		err := s.CopyMasterDataDir(streams, upgradedMasterBackupDir)
		if err != nil {
			return err
		}

		err = s.CopyMasterTablespaces(streams, utils.GetTablespaceDir()+string(os.PathSeparator))
		if err != nil {
			return err
		}

		return nil
	})

	st.Run(idl.Substep_UPGRADE_PRIMARIES, func(_ step.OutStreams) error {
		agentConns, err := s.AgentConns()

		if err != nil {
			return xerrors.Errorf("connect to gpupgrade agent: %w", err)
		}

		dataDirPair, err := s.GetDataDirPairs()

		if err != nil {
			return xerrors.Errorf("get source and target primary data directories: %w", err)
		}

		return UpgradePrimaries(UpgradePrimaryArgs{
			CheckOnly:              false,
			MasterBackupDir:        upgradedMasterBackupDir,
			AgentConns:             agentConns,
			DataDirPairMap:         dataDirPair,
			Source:                 s.Source,
			Target:                 s.Target,
			UseLinkMode:            s.UseLinkMode,
			TablespacesMappingFile: s.TablespacesMappingFilePath,
		})
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Start(streams)

		if err != nil {
			return xerrors.Errorf("failed to start target cluster: %w", err)
		}

		return nil
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Data: map[string]string{
		idl.ResponseKey_target_port.String():                  strconv.Itoa(s.Target.MasterPort()),
		idl.ResponseKey_target_master_data_directory.String(): s.Target.MasterDataDir(),
	}}}}
	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
