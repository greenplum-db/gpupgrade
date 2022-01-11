// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Initialize(req *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := step.Begin(idl.Step_INITIALIZE, stream, s.AgentConns)
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

	st.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(stream step.OutStreams) error {
		return FillConfiguration(s.Config, req, s.Connection, s.SaveConfig)
	})

	// Since the agents might not be up if gpupgrade is not properly installed, check it early on using ssh.
	st.RunInternalSubstep(func() error {
		return upgrade.EnsureGpupgradeVersionsMatch(AgentHosts(s.Source))
	})

	st.Run(idl.Substep_START_AGENTS, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, AgentHosts(s.Source), s.AgentPort, s.StateDir)
		return err
	})

	st.RunConditionally(idl.Substep_CHECK_DISK_SPACE, req.GetDiskFreeRatio() > 0, func(streams step.OutStreams) error {
		return CheckDiskSpace(streams, s.agentConns, req.GetDiskFreeRatio(), s.Source, s.Source.Tablespaces)
	})

	return st.Err()
}

func (s *Server) InitializeCreateCluster(req *idl.InitializeCreateClusterRequest, stream idl.CliToHub_InitializeCreateClusterServer) (err error) {
	st, err := step.Begin(idl.Step_INITIALIZE, stream, s.AgentConns)
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

	st.Run(idl.Substep_GENERATE_TARGET_CONFIG, func(_ step.OutStreams) error {
		return s.GenerateInitsystemConfig()
	})

	st.Run(idl.Substep_INIT_TARGET_CLUSTER, func(stream step.OutStreams) error {
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

	st.RunConditionally(idl.Substep_SETTING_DYNAMIC_LIBRARY_PATH_ON_TARGET_CLSUTER, req.GetDynamicLibraryPath() != upgrade.DefaultDynamicLibraryPath, func(stream step.OutStreams) error {
		return AppendDynamicLibraryPath(s.Intermediate, req.GetDynamicLibraryPath())
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(stream step.OutStreams) error {
		return s.Intermediate.Stop(stream)
	})

	st.Run(idl.Substep_BACKUP_TARGET_MASTER, func(stream step.OutStreams) error {
		sourceDir := s.Intermediate.MasterDataDir()
		targetDir := filepath.Join(s.StateDir, originalMasterBackupName)
		return RsyncMasterDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(stream step.OutStreams) error {
		return s.CheckUpgrade(stream, s.agentConns)
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
