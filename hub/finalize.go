// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Finalize(req *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	st, err := step.Begin(idl.Step_finalize, stream, s.AgentConns)
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

	st.RunInternalSubstep(func() error {
		return s.Intermediate.CheckActiveConnections()
	})

	st.RunConditionally(idl.Substep_upgrade_mirrors, s.Source.HasMirrors() && s.LinkMode, func(streams step.OutStreams) error {
		return UpgradeMirrorsUsingRsync(s.agentConns, s.Source, s.Intermediate, s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_upgrade_mirrors, s.Source.HasMirrors() && !s.LinkMode, func(streams step.OutStreams) error {
		return UpgradeMirrorsUsingGpAddMirrors(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_upgrade_standby, s.Source.HasStandby(), func(streams step.OutStreams) error {
		return UpgradeStandby(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.Run(idl.Substep_wait_for_cluster_to_be_ready_after_adding_mirrors_and_standby, func(streams step.OutStreams) error {
		return s.Intermediate.WaitForClusterToBeReady()
	})

	st.Run(idl.Substep_shutdown_target_cluster, func(streams step.OutStreams) error {
		return s.Intermediate.Stop(streams)
	})

	st.Run(idl.Substep_update_target_catalog, func(streams step.OutStreams) error {
		if err := s.Intermediate.StartCoordinatorOnly(streams); err != nil {
			return err
		}

		if err := UpdateCatalog(s.Intermediate, s.Target); err != nil {
			return err
		}

		return s.Intermediate.StopCoordinatorOnly(streams)
	})

	st.Run(idl.Substep_update_data_directories, func(_ step.OutStreams) error {
		return RenameDataDirectories(s.agentConns, s.Source, s.Intermediate)
	})

	st.Run(idl.Substep_update_target_conf_files, func(streams step.OutStreams) error {
		return UpdateConfFiles(s.agentConns, streams,
			s.Target.Version,
			s.Intermediate,
			s.Target,
		)
	})

	st.Run(idl.Substep_start_target_cluster, func(streams step.OutStreams) error {
		return s.Target.Start(streams)
	})

	st.Run(idl.Substep_wait_for_cluster_to_be_ready_after_updating_catalog, func(streams step.OutStreams) error {
		return s.Target.WaitForClusterToBeReady()
	})

	st.Run(idl.Substep_stop_target_cluster, func(streams step.OutStreams) error {
		return s.Target.Stop(streams)
	})

	var logArchiveDir string
	st.Run(idl.Substep_archive_log_directories, func(_ step.OutStreams) error {
		logArchiveDir, err = s.GetLogArchiveDir()
		if err != nil {
			return xerrors.Errorf("get log archive directory: %w", err)
		}

		return ArchiveLogDirectories(logArchiveDir, s.agentConns, s.Config.Target.CoordinatorHostname())
	})

	st.Run(idl.Substep_delete_segment_statedirs, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.CoordinatorHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_FinalizeResponse{
		FinalizeResponse: &idl.FinalizeResponse{
			TargetVersion:                          s.Target.Version.String(),
			LogArchiveDirectory:                    logArchiveDir,
			ArchivedSourceCoordinatorDataDirectory: s.Config.Intermediate.CoordinatorDataDir() + upgrade.OldSuffix,
			UpgradeID:                              s.Config.UpgradeID.String(),
			TargetCluster: &idl.Cluster{
				GPHome:                   s.Target.GPHome,
				Port:                     int32(s.Target.CoordinatorPort()),
				CoordinatorDataDirectory: s.Target.CoordinatorDataDir(),
			},
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
