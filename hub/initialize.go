// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Initialize(req *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := step.Begin(idl.Step_initialize, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			log.Printf("%s: %s", idl.Step_initialize, err)
		}
	}()

	st.Run(idl.Substep_saving_source_cluster_config, func(stream step.OutStreams) error {
		s.Config, err = config.GetInitializeConfiguration(s.Config.HubPort, req, false)
		if err != nil {
			return err
		}

		return s.Config.Save()
	})

	// Since the agents might not be up if gpupgrade is not properly installed, check it early on using ssh.
	st.RunInternalSubstep(func() error {
		return upgrade.EnsureGpupgradeVersionsMatch(AgentHosts(s.Source))
	})

	st.Run(idl.Substep_start_agents, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, AgentHosts(s.Source), s.AgentPort, s.StateDir)
		if err != nil {
			return err
		}

		_, err = s.AgentConns()
		if err != nil {
			return err
		}

		return nil
	})

	st.Run(idl.Substep_check_environment, func(streams step.OutStreams) error {
		return CheckEnvironment(append(AgentHosts(s.Source), s.Source.CoordinatorHostname()), s.Source.GPHome, s.Intermediate.GPHome)
	})

	st.Run(idl.Substep_create_backupdirs, func(streams step.OutStreams) error {
		// We do not combine the state directory and backup directory for
		// several reasons:
		// - The backup directory needs to be configurable since there
		// may not be enough space in the default location. If the state and
		// backup directories are combined and the backup directory needs to be
		// changed, then we have to preserve gpupgrade state by copying
		// substeps.json and config.json to the new location. This is awkward,
		// hard to manage, and error prone.
		// - The default state directory $HOME/.gpupgrade is known upfront with
		// no dependencies. Whereas the default backup directory is based on the
		// data directories. Having a state directory with no dependencies is
		// much easier to create and remove during the gpupgrade lifecycle.
		s.BackupDirs, err = ParseParentBackupDirs(req.GetParentBackupDirs(), s.Source)
		if err != nil {
			return err
		}

		err = s.Config.Save()
		if err != nil {
			return fmt.Errorf("save backup directories: %w", err)
		}

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
	st, err := step.Begin(idl.Step_initialize, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			log.Printf("%s: %s", idl.Step_initialize, err)
		}
	}()

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

		s.Intermediate.CatalogVersion = catalogVersion
		return s.Config.Save()
	})

	st.RunConditionally(idl.Substep_setting_dynamic_library_path_on_target_cluster, req.GetDynamicLibraryPath() != upgrade.DefaultDynamicLibraryPath, func(stream step.OutStreams) error {
		return AppendDynamicLibraryPath(s.Intermediate, req.GetDynamicLibraryPath())
	})

	st.Run(idl.Substep_shutdown_target_cluster, func(stream step.OutStreams) error {
		return s.Intermediate.Stop(stream)
	})

	st.Run(idl.Substep_backup_target_master, func(stream step.OutStreams) error {
		sourceDir := s.Intermediate.CoordinatorDataDir()
		targetDir := utils.GetCoordinatorPreUpgradeBackupDir(s.BackupDirs.CoordinatorBackupDir)

		return RsyncCoordinatorDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_check_upgrade, func(stream step.OutStreams) error {
		if err := UpgradeCoordinator(stream, s.BackupDirs.CoordinatorBackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode); err != nil {
			return err
		}

		return UpgradePrimaries(s.agentConns, s.BackupDirs.AgentHostsToBackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode)
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
