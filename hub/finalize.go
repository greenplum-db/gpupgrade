// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Finalize(_ *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
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
		mirrors := s.IntermediateTarget.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsMirror()
		})

		return UpgradeMirrors(s.StateDir, s.Connection, s.IntermediateTarget.MasterPort(),
			mirrors, greenplum.NewRunner(s.IntermediateTarget, streams), s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_UPGRADE_STANDBY, s.Source.HasStandby(), func(streams step.OutStreams) error {
		return UpgradeStandby(greenplum.NewRunner(s.IntermediateTarget, streams),
			StandbyConfig{
				Port:            s.IntermediateTarget.Standby().Port,
				Hostname:        s.IntermediateTarget.Standby().Hostname,
				DataDirectory:   s.IntermediateTarget.Standby().DataDir,
				UseHbaHostnames: s.UseHbaHostnames,
			})
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.IntermediateTarget.Stop(streams)
		if err != nil {
			return xerrors.Errorf("failed to stop target cluster: %w", err)
		}

		return nil
	})

	st.Run(idl.Substep_UPDATE_TARGET_CATALOG, func(streams step.OutStreams) error {
		if err := s.IntermediateTarget.StartMasterOnly(streams); err != nil {
			return err
		}

		if err := UpdateCatalog(s.Connection, s.IntermediateTarget, s.Target); err != nil {
			return err
		}

		return s.IntermediateTarget.StopMasterOnly(streams)
	})

	st.Run(idl.Substep_UPDATE_DATA_DIRECTORIES, func(_ step.OutStreams) error {
		return RenameDataDirectories(s.agentConns, s.Source, s.IntermediateTarget, s.UseLinkMode)
	})

	st.Run(idl.Substep_UPDATE_TARGET_CONF_FILES, func(streams step.OutStreams) error {
		return UpdateConfFiles(streams,
			s.Target.Version,
			s.Target.MasterDataDir(),
			s.IntermediateTarget.MasterPort(),
			s.Target.MasterPort(),
		)
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Start(streams)
		if err != nil {
			return xerrors.Errorf("failed to start target cluster: %w", err)
		}

		return nil
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
			ArchivedSourceMasterDataDirectory: s.Config.IntermediateTarget.MasterDataDir() + upgrade.OldSuffix,
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
