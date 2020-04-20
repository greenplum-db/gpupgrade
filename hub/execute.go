// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

const executeMasterBackupName = "upgraded-master.bak"

func (s *Server) Execute(request *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	upgradedMasterBackupDir := filepath.Join(s.StateDir, executeMasterBackupName)

	st, err := step.Begin(s.StateDir, "execute", stream)
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
		return s.CopyMasterDataDir(streams, upgradedMasterBackupDir)
	})

	st.Run(idl.Substep_UPGRADE_PRIMARIES, func(_ step.OutStreams) error {
		agentConns, err := s.AgentConns()

		if err != nil {
			return errors.Wrap(err, "failed to connect to gpupgrade agent")
		}

		dataDirPair, err := s.GetDataDirPairs()

		if err != nil {
			return errors.Wrap(err, "failed to get source and target primary data directories")
		}

		return UpgradePrimaries(UpgradePrimaryArgs{
			CheckOnly:       false,
			MasterBackupDir: upgradedMasterBackupDir,
			AgentConns:      agentConns,
			DataDirPairMap:  dataDirPair,
			Source:          s.Source,
			Target:          s.Target,
			UseLinkMode:     s.UseLinkMode,
		})
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Start(streams)

		if err != nil {
			return xerrors.Errorf("failed to start target cluster: %w", err)
		}

		return nil
	})

	message := MakeTargetClusterMessage(s.Target)
	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
