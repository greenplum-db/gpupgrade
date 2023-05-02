// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"log"
	"os/exec"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Revert(_ *idl.RevertRequest, stream idl.CliToHub_RevertServer) (err error) {
	st, err := step.Begin(idl.Step_revert, stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			log.Printf("%s: %s", idl.Step_revert, err)
		}
	}()

	hasExecuteStarted, err := step.HasStarted(idl.Step_execute)
	if err != nil {
		return err
	}

	if !s.Source.HasAllMirrorsAndStandby() && (s.Mode == idl.Mode_link) && hasExecuteStarted {
		return errors.New(`The source cluster does not have standby and/or mirrors and is being upgraded in link mode. Execute has started.
Cannot revert and restore the source cluster. Please contact support.`)
	}

	// If initialize exits early then only archiving the log directory and
	// deleting state directories need to be run.
	configCreated, err := step.HasCompleted(idl.Step_initialize, idl.Substep_saving_source_cluster_config)
	if err != nil {
		return err
	}

	agentsStarted, err := step.HasCompleted(idl.Step_initialize, idl.Substep_start_agents)
	if err != nil {
		return err
	}

	st.RunConditionally(idl.Substep_ensure_gpupgrade_agents_are_running, configCreated && agentsStarted, func(_ step.OutStreams) error {
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

	st.RunConditionally(idl.Substep_check_active_connections_on_target_cluster, configCreated, func(streams step.OutStreams) error {
		return s.Intermediate.CheckActiveConnections(streams)
	})

	st.RunConditionally(idl.Substep_shutdown_target_cluster, configCreated, func(streams step.OutStreams) error {
		return s.Intermediate.Stop(streams)
	})

	st.RunConditionally(idl.Substep_delete_target_cluster_datadirs, configCreated, func(streams step.OutStreams) error {
		return DeleteCoordinatorAndPrimaryDataDirectories(streams, s.agentConns, s.Intermediate)
	})

	st.RunConditionally(idl.Substep_delete_tablespaces, configCreated, func(streams step.OutStreams) error {
		return DeleteTargetTablespaces(streams, s.agentConns, s.Config.Intermediate, s.Intermediate.CatalogVersion, s.Source.Tablespaces)
	})

	// See "Reverting to old cluster" from https://www.postgresql.org/docs/9.4/pgupgrade.html
	st.RunConditionally(idl.Substep_restore_pgcontrol, configCreated && s.Mode == idl.Mode_link, func(streams step.OutStreams) error {
		return RestoreCoordinatorAndPrimariesPgControl(streams, s.agentConns, s.Source)
	})

	st.RunConditionally(idl.Substep_restore_source_cluster, configCreated && s.Mode == idl.Mode_link && s.Source.HasAllMirrorsAndStandby(), func(stream step.OutStreams) error {
		if err := RsyncCoordinatorAndPrimaries(stream, s.agentConns, s.Source); err != nil {
			return err
		}

		return RsyncCoordinatorAndPrimariesTablespaces(stream, s.agentConns, s.Source)
	})

	primariesUpgraded, err := step.HasRun(idl.Step_execute, idl.Substep_upgrade_primaries)
	if err != nil {
		return err
	}

	// Due to a GPDB 5X issue upgrading the primaries results in an invalid
	// checkpoint upon starting. The checkpoint needs to be replicated to the
	// mirrors with rsync or gprecoverseg. When upgrading the mirrors during
	// finalize the checkpoint is replicated. In copy mode the 5X source cluster
	// mirrors do not start causing gpstart to return a non-zero exit status.
	// Ignore such failures, as gprecoverseg is executed to bring up the mirrors.
	// Running gprecoverseg is expected to not take long.
	shouldHandle5XMirrorFailure := s.Source.Version.Major == 5 && s.Mode != idl.Mode_link && primariesUpgraded

	st.RunConditionally(idl.Substep_start_source_cluster, configCreated, func(streams step.OutStreams) error {
		err = s.Source.Start(streams)
		var exitErr *exec.ExitError
		if xerrors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 && shouldHandle5XMirrorFailure {
				return nil
			}
		}

		if err != nil {
			return err
		}

		return nil
	})

	st.RunConditionally(idl.Substep_recoverseg_source_cluster, configCreated && shouldHandle5XMirrorFailure, func(streams step.OutStreams) error {
		return Recoverseg(streams, s.Source, s.UseHbaHostnames)
	})

	var logArchiveDir string
	st.AlwaysRun(idl.Substep_archive_log_directories, func(_ step.OutStreams) error {
		logDir, err := utils.GetLogDir()
		if err != nil {
			return err
		}

		logArchiveDir = GetLogArchiveDir(logDir, s.UpgradeID, time.Now())
		return ArchiveLogDirectories(logDir, logArchiveDir, s.agentConns, s.Config.Source.CoordinatorHostname())
	})

	st.RunConditionally(idl.Substep_delete_backupdir, configCreated, func(streams step.OutStreams) error {
		return DeleteBackupDirectories(streams, s.agentConns, s.BackupDirs)
	})

	st.AlwaysRun(idl.Substep_delete_segment_statedirs, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.CoordinatorHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_RevertResponse{
		RevertResponse: &idl.RevertResponse{
			LogArchiveDirectory: logArchiveDir,
			Source: &idl.Cluster{
				Destination: idl.ClusterDestination_source,
				GpHome:      s.Source.GPHome,
				Version:     s.Source.Version.String(),
				Coordinator: &idl.Segment{
					DbID:      int32(s.Source.Coordinator().DbID),
					ContentID: int32(s.Source.Coordinator().ContentID),
					Role:      idl.Segment_Role(idl.Segment_Role_value[s.Source.Coordinator().Role]),
					Port:      int32(s.Source.CoordinatorPort()),
					Hostname:  s.Source.CoordinatorHostname(),
					DataDir:   s.Source.CoordinatorDataDir(),
				},
			},
		},
	}}}}

	if err := stream.Send(message); err != nil {
		return xerrors.Errorf("sending response message: %w", err)
	}

	return st.Err()
}
