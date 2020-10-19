// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const connectionString = "postgresql://localhost:%d/template1?gp_session_role=utility&search_path="

func (s *Server) Initialize(in *idl.InitializeRequest, stream idl.CliToHub_InitializeServer) (err error) {
	st, err := step.Begin(s.StateDir, idl.Step_INITIALIZE, stream)
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

	st.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(stream step.OutStreams) error {
		conn, err := sql.Open("pgx", fmt.Sprintf(connectionString, in.SourcePort))
		if err != nil {
			return err
		}
		defer func() {
			if cerr := conn.Close(); cerr != nil {
				err = errorlist.Append(err, cerr)
			}
		}()

		return FillConfiguration(s.Config, conn, stream, in, s.SaveConfig)
	})

	st.Run(idl.Substep_START_AGENTS, func(_ step.OutStreams) error {
		_, err := RestartAgents(context.Background(), nil, AgentHosts(s.Source), s.AgentPort, s.StateDir)
		return err
	})

	return st.Err()
}

func (s *Server) InitializeCreateCluster(in *idl.InitializeCreateClusterRequest, stream idl.CliToHub_InitializeCreateClusterServer) (err error) {
	st, err := step.Begin(s.StateDir, idl.Step_INITIALIZE, stream)
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
		err := s.CreateTargetCluster(stream)
		if err != nil {
			return err
		}

		// Persist target catalog version which is needed to revert tablespaces.
		// We do this right after target cluster creation since during revert the
		// state of the cluster is unknown.
		version, err := GetCatalogVersion(stream, s.Target.GPHome, s.Target.MasterDataDir())
		if err != nil {
			return err
		}

		s.TargetCatalogVersion = version
		return s.SaveConfig()
	})

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(stream step.OutStreams) error {
		err := s.Target.Stop(stream)

		if err != nil {
			return xerrors.Errorf("failed to stop target cluster: %w", err)
		}

		return nil
	})

	st.Run(idl.Substep_BACKUP_TARGET_MASTER, func(stream step.OutStreams) error {
		sourceDir := s.Target.MasterDataDir()
		targetDir := filepath.Join(s.StateDir, originalMasterBackupName)
		return RsyncMasterDataDir(stream, sourceDir, targetDir)
	})

	st.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(stream step.OutStreams) error {
		conns, err := s.AgentConns()

		if err != nil {
			return err
		}

		return s.CheckUpgrade(stream, conns)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{
		Contents: &idl.Response_InitializeResponse{InitializeResponse: &idl.InitializeResponse{
			HasMirrors: s.Config.Source.HasMirrors(),
			HasStandby: s.Config.Source.HasStandby(),
		}}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
