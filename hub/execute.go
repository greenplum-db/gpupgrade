// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"log"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Execute(req *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	st, err := step.Begin(idl.Step_execute, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			log.Printf("%s: %s", idl.Step_execute, err)
		}
	}()

	st.Run(idl.Substep_check_active_connections_on_source_cluster, func(streams step.OutStreams) error {
		return s.Source.CheckActiveConnections(streams)
	})

	st.Run(idl.Substep_shutdown_source_cluster, func(streams step.OutStreams) error {
		return s.Source.Stop(streams)
	})

	st.Run(idl.Substep_upgrade_master, func(streams step.OutStreams) error {
		return UpgradeCoordinator(streams, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_upgrade, s.LinkMode)
	})

	st.Run(idl.Substep_copy_master, func(streams step.OutStreams) error {
		err := CopyCoordinatorDataDir(streams, s.Intermediate.CoordinatorDataDir(), utils.GetCoordinatorPostUpgradeBackupDir(), s.Intermediate.PrimaryHostnames())
		if err != nil {
			return err
		}

		return CopyCoordinatorTablespaces(streams, s.Source.Tablespaces, utils.GetTablespaceDir(), s.Intermediate.PrimaryHostnames())
	})

	st.Run(idl.Substep_upgrade_primaries, func(streams step.OutStreams) error {
		return UpgradePrimaries(s.agentConns, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_upgrade, s.LinkMode)
	})

	st.Run(idl.Substep_start_target_cluster, func(streams step.OutStreams) error {
		return s.Intermediate.Start(streams)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_ExecuteResponse{
		ExecuteResponse: &idl.ExecuteResponse{
			Target: &idl.Cluster{
				GPHome:                   s.Intermediate.GPHome,
				CoordinatorDataDirectory: s.Intermediate.CoordinatorDataDir(),
				Port:                     int32(s.Intermediate.CoordinatorPort()),
			}},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
