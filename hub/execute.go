// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"log"
	"path/filepath"

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
		return UpgradeCoordinator(streams, s.BackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_upgrade, s.Mode)
	})

	st.Run(idl.Substep_copy_master, func(streams step.OutStreams) error {
		// The execute backup directory flag takes precedence over the value set
		// during initialize. The execute flag is used as an emergency stop gap
		// to set the backup directory where it is used without needing to
		// revert and re-run initialize and execute.
		if req.GetParentBackupDir() != "" {
			err = DeleteBackupDirectories(streams, s.agentConns, s.BackupDir)
			if err != nil {
				return err
			}

			s.BackupDir = filepath.Join(req.GetParentBackupDir(), ".gpupgrade")
			err = s.SaveConfig()
			if err != nil {
				return fmt.Errorf("save backup directory: %w", err)
			}

			err = CreateBackupDirectories(streams, s.agentConns, s.BackupDir)
			if err != nil {
				return err
			}
		}

		nextAction := `Consider setting an alternative backup directory with
"gpupgrade execute --verbose --parent-backup-dir /tmp/backup"

This sets the location used internally to store a backup of the master data 
directory and user defined master tablespaces. It defaults to the root directory 
of the master data directory such as /data given /data/master/gpseg-1.`

		err := CopyCoordinatorDataDir(streams, s.Intermediate.CoordinatorDataDir(), utils.GetCoordinatorPostUpgradeBackupDir(s.BackupDir), s.Intermediate.PrimaryHostnames())
		if err != nil {
			return utils.NewNextActionErr(err, nextAction)
		}

		err = CopyCoordinatorTablespaces(streams, s.Source.Version, s.Source.Tablespaces, utils.GetTablespaceBackupDir(s.BackupDir), s.Intermediate.PrimaryHostnames())
		if err != nil {
			return utils.NewNextActionErr(err, nextAction)
		}

		return nil
	})

	st.Run(idl.Substep_upgrade_primaries, func(streams step.OutStreams) error {
		return UpgradePrimaries(s.agentConns, s.BackupDir, req.PgUpgradeVerbose, s.Source, s.Intermediate, idl.PgOptions_upgrade, s.Mode)
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
