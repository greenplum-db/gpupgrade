// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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
		return FillConfiguration(s.Config, req, s.SaveConfig)
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

	st.Run(idl.Substep_create_backupdir, func(streams step.OutStreams) error {
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
		// coordinator data directory. Having a state directory with no
		// dependencies is much easier to create and remove during the gpupgrade
		// lifecycle.

		parentBackupDir := req.GetParentBackupDir()
		if parentBackupDir == "" {
			// Default to the root directory of the master data directory such
			// as /data given /data/master/gpseg-1. The backup directory will be
			// /data/.gpupgrade on all hosts.
			//
			// NOTE: We do not use the parent directory of the coordinator data
			// directory such as /data/master since that is less likely to be
			// the same across all hosts.
			parts := strings.SplitAfterN(s.Source.CoordinatorDataDir(), string(os.PathSeparator), 3)
			parentBackupDir = filepath.Clean(filepath.Join(parts[0], parts[1]))
		}

		s.BackupDir = filepath.Join(parentBackupDir, ".gpupgrade")
		err = s.SaveConfig()
		if err != nil {
			return fmt.Errorf("save backup directory: %w", err)
		}

		err = CreateBackupDirectories(streams, s.agentConns, s.BackupDir)
		if err != nil {
			nextAction := `1. Run "gpupgrade revert"

2. Consider specifying the "parent_backup_dir" parameter in gpupgrade_config.
This sets the location used internally to store the backup of the master 
data directory and user defined master tablespaces. It defaults to the root 
directory of the master data directory such as /data given /data/master/gpseg-1.

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
		return s.SaveConfig()
	})

	st.RunConditionally(idl.Substep_setting_dynamic_library_path_on_target_cluster, req.GetDynamicLibraryPath() != upgrade.DefaultDynamicLibraryPath, func(stream step.OutStreams) error {
		return AppendDynamicLibraryPath(s.Intermediate, req.GetDynamicLibraryPath())
	})

	st.Run(idl.Substep_shutdown_target_cluster, func(stream step.OutStreams) error {
		return s.Intermediate.Stop(stream)
	})

	st.Run(idl.Substep_backup_target_master, func(stream step.OutStreams) error {
		sourceDir := s.Intermediate.CoordinatorDataDir()
		targetDir := utils.GetCoordinatorPreUpgradeBackupDir(s.BackupDir)

		return RsyncCoordinatorDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_check_upgrade, func(stream step.OutStreams) error {
		if err := UpgradeCoordinator(stream, s.BackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode); err != nil {
			return err
		}

		return UpgradePrimaries(s.agentConns, s.BackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_check, s.Mode)
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
