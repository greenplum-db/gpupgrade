// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/greenplum"
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
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	st.RunInternalSubstep(func() error {
		sourceVersion, err := greenplum.Version(req.SourceGPHome)
		if err != nil {
			return err
		}

		targetVersion, err := greenplum.Version(req.TargetGPHome)
		if err != nil {
			return err
		}

		conn := greenplum.Connection(semver.MustParse(sourceVersion), semver.MustParse(targetVersion))
		s.Connection = conn

		return nil
	})

	st.Run(idl.Substep_saving_source_cluster_config, func(stream step.OutStreams) error {
		return FillConfiguration(s.Config, req, s.Connection, s.SaveConfig)
	})

	// Since the agents might not be up if gpupgrade is not properly installed, check it early on using ssh.
	st.RunInternalSubstep(func() error {
		return upgrade.EnsureGpupgradeVersionsMatch(AgentHosts(s.Source))
	})

	st.Run(idl.Substep_start_agents, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, AgentHosts(s.Source), s.AgentPort, s.StateDir)
		return err
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
			gplog.Error(fmt.Sprintf("initialize: %s", err))
		}
	}()

	st.Run(idl.Substep_generate_target_config, func(_ step.OutStreams) error {
		return s.GenerateInitsystemConfig()
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
		targetDir := utils.GetCoordinatorPreUpgradeBackupDir()

		err := utils.System.MkdirAll(targetDir, 0700)
		if err != nil {
			return err
		}

		return RsyncCoordinatorDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_check_upgrade, func(stream step.OutStreams) error {
		if err := UpgradeCoordinator(stream, s.Source, s.Intermediate, idl.PgOptions_check, s.LinkMode); err != nil {
			return err
		}

		return UpgradePrimaries(s.agentConns, s.Source, s.Intermediate, idl.PgOptions_check, s.LinkMode)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_InitializeResponse{
		InitializeResponse: &idl.InitializeResponse{
			HasMirrors: s.Config.Source.HasMirrors(),
			HasStandby: s.Config.Source.HasStandby(),
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
