// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) Initialize(req *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := step.Begin(idl.Step_initialize, stream)
	if err != nil {
		return err
	}

	// Since the agents might not be up if gpupgrade is not properly installed, check it early on using ssh.
	st.Run(idl.Substep_verify_gpupgrade_is_installed_across_all_hosts, func(streams step.OutStreams) error {
		return upgrade.EnsureGpupgradeVersionsMatch(AgentHosts(s.Source))
	})

	st.AlwaysRun(idl.Substep_start_agents, func(_ step.OutStreams) error {
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

	st.AlwaysRun(idl.Substep_check_environment, func(streams step.OutStreams) error {
		return CheckEnvironment(append(AgentHosts(s.Source), s.Source.CoordinatorHostname()), s.Source.GPHome, s.Intermediate.GPHome)
	})

	st.Run(idl.Substep_create_backupdirs, func(streams step.OutStreams) error {
		err = CreateBackupDirectories(streams, s.agentConns, s.BackupDirs)
		if err != nil {
			nextAction := `1. Run "gpupgrade revert"

2. Consider setting the "parent_backup_dirs" parameter in gpupgrade_config.

This sets the internal location to store the backup of the master data directory 
and user defined master tablespaces. It defaults to the parent directory of each 
primary data directory on each host including the standby. For example, 
/data/coordinator given /data/coordinator/gpseg-1, and 
/data1/primaries given /data1/primaries/gpseg1.

The parent_backup_dirs parameter accepts either a single directory or multiple 
host:directory pairs. To specify a single directory across all hosts set a 
single directory such as /dir. To specify different directories for each host 
use the form "host1:/dir1,host2:/dir2,host3:/dir3" where the first host must be 
the master.

3. Re-run "gupgrade initialize"`
			return utils.NewNextActionErr(err, nextAction)
		}

		// The following CheckDiskSpace encapsulates the backup directory since
		// it usually is on the same filesystem as the data directories.
		return nil
	})

	st.RunConditionally(idl.Substep_check_disk_space, req.GetDiskFreeRatio() > 0, func(streams step.OutStreams) error {
		return CheckDiskSpace(streams, s.agentConns, req.GetDiskFreeRatio(), s.Source, s.Source.Tablespaces)
	})

	return st.Err()
}

func (s *Server) InitializeCreateCluster(req *idl.InitializeCreateClusterRequest, stream idl.CliToHub_InitializeCreateClusterServer) (err error) {
	st, err := step.Begin(idl.Step_initialize, stream)
	if err != nil {
		return err
	}

	st.Run(idl.Substep_generate_target_config, func(_ step.OutStreams) error {
		return s.GenerateInitsystemConfig(s.Source)
	})

	st.Run(idl.Substep_init_target_cluster, func(stream step.OutStreams) error {
		err := s.RemoveIntermediateCluster(stream)
		if err != nil {
			return err
		}

		err = InitTargetCluster(stream, s.Intermediate)
		if err != nil {
			return err
		}

		// Persist target catalog version which is needed to revert tablespaces.
		// We do this right after target cluster creation since during revert the
		// state of the cluster is unknown.
		catalogVersion, err := GetCatalogVersion(s.Intermediate)
		if err != nil {
			return err
		}

		s.Config.Intermediate.CatalogVersion = catalogVersion
		return s.Config.Write()
	})

	st.RunConditionally(idl.Substep_setting_dynamic_library_path_on_target_cluster, req.GetDynamicLibraryPath() != upgrade.DefaultDynamicLibraryPath, func(stream step.OutStreams) error {
		return AppendDynamicLibraryPath(s.Intermediate, req.GetDynamicLibraryPath())
	})

	st.AlwaysRun(idl.Substep_shutdown_target_cluster, func(stream step.OutStreams) error {
		return s.Intermediate.Stop(stream)
	})

	st.Run(idl.Substep_backup_target_master, func(stream step.OutStreams) error {
		sourceDir := s.Intermediate.CoordinatorDataDir()
		targetDir := utils.GetCoordinatorPreUpgradeBackupDir(s.BackupDirs.CoordinatorBackupDir)

		return RsyncCoordinatorDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_initialize_wait_for_cluster_to_be_ready, func(streams step.OutStreams) error {
		return s.Source.WaitForClusterToBeReady()
	})

	st.AlwaysRun(idl.Substep_check_upgrade, func(stream step.OutStreams) error {
		if req.GetSkipPgUpgradeChecks() {
			log.Print("skipping pg_upgrade checks")
			return nil
		}

		if err := UpgradeCoordinator(stream, s.BackupDirs.CoordinatorBackupDir, req.GetPgUpgradeVerbose(), req.GetSkipPgUpgradeChecks(), s.PgUpgradeJobs, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode); err != nil {
			return err
		}

		return UpgradePrimaries(s.agentConns, s.BackupDirs.AgentHostsToBackupDir, req.GetPgUpgradeVerbose(), req.GetSkipPgUpgradeChecks(), s.PgUpgradeJobs, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_InitializeResponse{
		InitializeResponse: &idl.InitializeResponse{
			HasAllMirrorsAndStandby: s.Config.Source.HasAllMirrorsAndStandby(),
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
