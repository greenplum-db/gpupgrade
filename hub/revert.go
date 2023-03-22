// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"log"
	"os/exec"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Revert(_ *idl.RevertRequest, stream idl.CliToHub_RevertServer) (err error) {
	st, err := step.Begin(idl.Step_revert, stream, s.AgentConns)
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

	hasInitializeStarted, err := step.HasCompleted(idl.Step_initialize, idl.Substep_saving_source_cluster_config)
	if err != nil {
		return err
	}

	// If CLI Initialize exited before the InitializeRequest was sent
	// to the hub, we will only need to do a couple revert substeps.
	if !hasInitializeStarted {
		s.Config, err = GetEarlyInitializeConfiguration(s.Port, s.Source.CoordinatorPort(), s.Source.GPHome)
		if err != nil {
			return err
		}

		st.OnlyRun(
			idl.Substep_archive_log_directories,
			idl.Substep_delete_segment_statedirs,
		)
	}

	st.RunConditionally(idl.Substep_check_active_connections_on_target_cluster, s.Intermediate != nil, func(streams step.OutStreams) error {
		return s.Intermediate.CheckActiveConnections(streams)
	})

	st.RunConditionally(idl.Substep_shutdown_target_cluster, s.Intermediate != nil, func(streams step.OutStreams) error {
		return s.Intermediate.Stop(streams)
	})

	st.RunConditionally(idl.Substep_delete_target_cluster_datadirs,
		s.Intermediate != nil && s.Intermediate.Primaries != nil && s.Intermediate.CoordinatorDataDir() != "",
		func(streams step.OutStreams) error {
			return DeleteCoordinatorAndPrimaryDataDirectories(streams, s.agentConns, s.Intermediate)
		})

	st.RunConditionally(idl.Substep_delete_tablespaces,
		s.Intermediate != nil && s.Intermediate.Primaries != nil && s.Intermediate.CoordinatorDataDir() != "",
		func(streams step.OutStreams) error {
			return DeleteTargetTablespaces(streams, s.agentConns, s.Config.Intermediate, s.Intermediate.CatalogVersion, s.Source.Tablespaces)
		})

	// See "Reverting to old cluster" from https://www.postgresql.org/docs/9.4/pgupgrade.html
	st.RunConditionally(idl.Substep_restore_pgcontrol, s.Mode == idl.Mode_link, func(streams step.OutStreams) error {
		return RestoreCoordinatorAndPrimariesPgControl(streams, s.agentConns, s.Source)
	})

	st.RunConditionally(idl.Substep_restore_source_cluster, s.Mode == idl.Mode_link && s.Source.HasAllMirrorsAndStandby(), func(stream step.OutStreams) error {
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

	st.Run(idl.Substep_start_source_cluster, func(streams step.OutStreams) error {
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

	st.RunConditionally(idl.Substep_recoverseg_source_cluster, shouldHandle5XMirrorFailure, func(streams step.OutStreams) error {
		return Recoverseg(streams, s.Source, s.UseHbaHostnames)
	})

	var logArchiveDir string
	st.Run(idl.Substep_archive_log_directories, func(_ step.OutStreams) error {
		logArchiveDir, err = s.GetLogArchiveDir()
		if err != nil {
			return xerrors.Errorf("get log archive directory: %w", err)
		}

		return ArchiveLogDirectories(logArchiveDir, s.agentConns, s.Config.Source.CoordinatorHostname())
	})

	st.Run(idl.Substep_delete_backupdir, func(streams step.OutStreams) error {
		return DeleteBackupDirectories(streams, s.agentConns, s.BackupDirs)
	})

	st.Run(idl.Substep_delete_segment_statedirs, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.CoordinatorHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_RevertResponse{
		RevertResponse: &idl.RevertResponse{
			SourceVersion:       s.Source.Version.String(),
			LogArchiveDirectory: logArchiveDir,
			Source: &idl.Cluster{
				GPHome:                   s.Source.GPHome,
				CoordinatorDataDirectory: s.Source.CoordinatorDataDir(),
				Port:                     int32(s.Source.CoordinatorPort()),
			},
		},
	}}}}

	if err := stream.Send(message); err != nil {
		return xerrors.Errorf("sending response message: %w", err)
	}

	return st.Err()
}
