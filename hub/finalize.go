// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Finalize(req *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	st, err := step.Begin(idl.Step_FINALIZE, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("finalize: %s", err))
		}
	}()

	// In link mode to reduce disk space remove the source mirror and standby data directories and tablespaces.
	st.RunConditionally(idl.Substep_REMOVE_SOURCE_MIRRORS, s.UseLinkMode, func(streams step.OutStreams) error {
		if err := DeleteMirrorAndStandbyDataDirectories(s.agentConns, s.Source); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror segment data directories: %w", err)
		}

		if err := DeleteSourceTablespacesOnMirrorsAndStandby(s.agentConns, s.Source, s.Tablespaces); err != nil {
			return xerrors.Errorf("removing source cluster standby and mirror tablespace data directories: %w", err)
		}

		return nil
	})

	st.RunConditionally(idl.Substep_UPGRADE_MIRRORS, s.Source.HasMirrors(), func(streams step.OutStreams) error {
		return UpgradeMirrors(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_UPGRADE_STANDBY, s.Source.HasStandby(), func(streams step.OutStreams) error {
		return UpgradeStandby(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.Run(idl.Substep_WAIT_FOR_CLUSTER_TO_BE_READY_AFTER_ADDING_MIRRORS_AND_STANDBY, func(streams step.OutStreams) error {
		return s.Intermediate.WaitForClusterToBeReady(s.Connection)
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
		return s.Intermediate.Stop(streams)
	})

	st.Run(idl.Substep_UPDATE_TARGET_CATALOG, func(streams step.OutStreams) error {
		if err := s.Intermediate.StartMasterOnly(streams); err != nil {
			return err
		}

		if err := UpdateCatalog(s.Connection, s.Intermediate, s.Target); err != nil {
			return err
		}

		return s.Intermediate.StopMasterOnly(streams)
	})

	st.Run(idl.Substep_UPDATE_DATA_DIRECTORIES, func(_ step.OutStreams) error {
		return RenameDataDirectories(s.agentConns, s.Source, s.Intermediate, s.UseLinkMode)
	})

	st.Run(idl.Substep_UPDATE_TARGET_CONF_FILES, func(streams step.OutStreams) error {
		return UpdateConfFiles(s.agentConns, streams,
			s.Target.Version,
			s.Intermediate,
			s.Target,
		)
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		return s.Target.Start(streams)
	})

	st.Run(idl.Substep_WAIT_FOR_CLUSTER_TO_BE_READY_AFTER_UPDATING_CATALOG, func(streams step.OutStreams) error {
		return s.Target.WaitForClusterToBeReady(s.Connection)
	})

	logArchiveDir, err := s.GetLogArchiveDir()
	if err != nil {
		return fmt.Errorf("getting archive directory: %w", err)
	}

	st.Run(idl.Substep_ARCHIVE_LOG_DIRECTORIES, func(_ step.OutStreams) error {
		// Archive log directory on master
		logDir, err := utils.GetLogDir()
		if err != nil {
			return err
		}

		gplog.Debug("archiving log directory %q to %q", logDir, logArchiveDir)
		if err = utils.Move(logDir, logArchiveDir); err != nil {
			return err
		}

		return ArchiveSegmentLogDirectories(s.agentConns, s.Config.Target.MasterHostname(), logArchiveDir)
	})

	st.Run(idl.Substep_DELETE_SEGMENT_STATEDIRS, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.MasterHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_FinalizeResponse{
		FinalizeResponse: &idl.FinalizeResponse{
			TargetVersion:                     s.Target.Version.String(),
			LogArchiveDirectory:               logArchiveDir,
			ArchivedSourceMasterDataDirectory: s.Config.Intermediate.MasterDataDir() + upgrade.OldSuffix,
			UpgradeID:                         s.Config.UpgradeID.String(),
			Target: &idl.Cluster{
				Port:                int32(s.Target.MasterPort()),
				MasterDataDirectory: s.Target.MasterDataDir(),
			},
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
