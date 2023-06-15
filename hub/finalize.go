// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Finalize(req *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	st, err := step.Begin(idl.Step_finalize, stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			log.Printf("%s: %s", idl.Step_finalize, err)
		}
	}()

	st.AlwaysRun(idl.Substep_ensure_gpupgrade_agents_are_running, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, AgentHosts(s.Source), s.AgentPort, utils.GetStateDir())
		if err != nil {
			return err
		}

		_, err = s.AgentConns()
		if err != nil {
			return err
		}

		return nil
	})

	st.AlwaysRun(idl.Substep_check_active_connections_on_target_cluster, func(streams step.OutStreams) error {
		return s.Intermediate.CheckActiveConnections(streams)
	})

	st.RunConditionally(idl.Substep_upgrade_mirrors, s.Source.HasMirrors() && s.Mode == idl.Mode_link, func(streams step.OutStreams) error {
		return UpgradeMirrorsUsingRsync(s.agentConns, s.Source, s.Intermediate, s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_upgrade_mirrors, s.Source.HasMirrors() && s.Mode != idl.Mode_link, func(streams step.OutStreams) error {
		return UpgradeMirrorsUsingGpAddMirrors(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.RunConditionally(idl.Substep_upgrade_standby, s.Source.HasStandby(), func(streams step.OutStreams) error {
		return UpgradeStandby(streams, s.Intermediate, s.UseHbaHostnames)
	})

	st.Run(idl.Substep_wait_for_cluster_to_be_ready_after_adding_mirrors_and_standby, func(streams step.OutStreams) error {
		return s.Intermediate.WaitForClusterToBeReady()
	})

	st.AlwaysRun(idl.Substep_shutdown_target_cluster, func(streams step.OutStreams) error {
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

	st.AlwaysRun(idl.Substep_start_target_cluster, func(streams step.OutStreams) error {
		return s.Target.Start(streams)
	})

	st.AlwaysRun(idl.Substep_wait_for_cluster_to_be_ready_after_updating_catalog, func(streams step.OutStreams) error {
		return s.Target.WaitForClusterToBeReady()
	})

	st.Run(idl.Substep_analyze_target_cluster, func(streams step.OutStreams) error {
		upgradeDir, err := utils.GetPgUpgradeDir(s.Target.Coordinator().Role, int32(s.Target.Coordinator().ContentID))
		if err != nil {
			return err
		}

		return s.Target.RunCmd(streams, filepath.Join(upgradeDir, "analyze_new_cluster.sh"))
	})

	var logArchiveDir string
	st.AlwaysRun(idl.Substep_archive_log_directories, func(_ step.OutStreams) error {
		logDir, err := utils.GetLogDir()
		if err != nil {
			return err
		}

		logArchiveDir = GetLogArchiveDir(logDir, s.UpgradeID, time.Now())
		return ArchiveLogDirectories(logDir, logArchiveDir, s.agentConns, s.Config.Target.CoordinatorHostname())
	})

	st.Run(idl.Substep_delete_backupdir, func(streams step.OutStreams) error {
		return DeleteBackupDirectories(streams, s.agentConns, s.BackupDirs)
	})

	st.AlwaysRun(idl.Substep_delete_segment_statedirs, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.CoordinatorHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_FinalizeResponse{
		FinalizeResponse: &idl.FinalizeResponse{
			LogArchiveDirectory: logArchiveDir,
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
