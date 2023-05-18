// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"log"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Execute(req *idl.ExecuteRequest, stream idl.CliToHub_ExecuteServer) (err error) {
	st, err := step.Begin(idl.Step_execute, stream)
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

	st.AlwaysRun(idl.Substep_ensure_gpupgrade_agents_are_running, func(_ step.OutStreams) error {
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

	st.AlwaysRun(idl.Substep_check_active_connections_on_source_cluster, func(streams step.OutStreams) error {
		return s.Source.CheckActiveConnections(streams)
	})

	// We do not always run this cluster synchronization check
	// because checking requires the source cluster to be available
	// and Substep_upgrade_master makes the source cluster
	// unavailable.
	st.Run(idl.Substep_wait_for_cluster_to_be_ready_before_upgrade_master, func(streams step.OutStreams) error {
		if err := s.Source.Start(streams); err != nil {
			return err
		}

		return s.Source.WaitForClusterToBeReady()
	})

	st.AlwaysRun(idl.Substep_shutdown_source_cluster, func(streams step.OutStreams) error {
		return s.Source.Stop(streams)
	})

	st.Run(idl.Substep_upgrade_master, func(streams step.OutStreams) error {
		return UpgradeCoordinator(streams, s.BackupDirs.CoordinatorBackupDir, req.GetPgUpgradeVerbose(), req.GetSkipPgUpgradeChecks(), s.Source, s.Intermediate, idl.PgOptions_upgrade, s.Mode)
	})

	st.Run(idl.Substep_copy_master, func(streams step.OutStreams) error {
		// The execute backup directory flag takes precedence over the value set
		// during initialize. The execute flag is used as an emergency stop gap
		// to set the backup directory where it is used without needing to
		// revert and re-run initialize and execute.
		if req.GetParentBackupDirs() != "" {
			err = DeleteBackupDirectories(streams, s.agentConns, s.BackupDirs)
			if err != nil {
				return err
			}

			s.Config.BackupDirs, err = backupdir.ParseParentBackupDirs(req.GetParentBackupDirs(), *s.Source)
			if err != nil {
				return err
			}

			err = s.Config.Write()
			if err != nil {
				return fmt.Errorf("save backup directories: %w", err)
			}

			err = CreateBackupDirectories(streams, s.agentConns, s.BackupDirs)
			if err != nil {
				return err
			}
		}

		nextAction := `Consider setting an alternative backup directory with
"gpupgrade execute --verbose --parent-backup-dirs /data"

The parent_backup_dirs parameter sets the internal location to store the backup 
of the master data directory and user defined master tablespaces. It defaults 
to the parent directory of each primary data directory on each host including 
the standby. For example, /data/coordinator given /data/coordinator/gpseg-1, and 
/data1/primaries given /data1/primaries/gpseg1.

The parent_backup_dirs parameter accepts either a single directory or multiple 
host:directory pairs. To specify a single directory across all hosts set a 
single directory such as /dir. To specify different directories for each host 
use the form "host1:/dir1,host2:/dir2,host3:/dir3" where the first host must be 
the master.`

		err := CopyCoordinatorDataDir(streams, s.Intermediate.CoordinatorDataDir(), s.BackupDirs.AgentHostsToBackupDir)
		if err != nil {
			return utils.NewNextActionErr(err, nextAction)
		}

		err = CopyCoordinatorTablespaces(streams, s.Source.Version, s.Source.Tablespaces, s.BackupDirs.AgentHostsToBackupDir)
		if err != nil {
			return utils.NewNextActionErr(err, nextAction)
		}

		return nil
	})

	st.Run(idl.Substep_upgrade_primaries, func(streams step.OutStreams) error {
		return UpgradePrimaries(s.agentConns, s.BackupDirs.AgentHostsToBackupDir, req.GetPgUpgradeVerbose(), req.GetSkipPgUpgradeChecks(), s.Source, s.Intermediate, idl.PgOptions_upgrade, s.Mode)
	})

	st.AlwaysRun(idl.Substep_start_target_cluster, func(streams step.OutStreams) error {
		return s.Intermediate.Start(streams)
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_ExecuteResponse{
		ExecuteResponse: &idl.ExecuteResponse{
			Target: &idl.Cluster{
				Destination: idl.ClusterDestination_target,
				GpHome:      s.Intermediate.GPHome,
				Version:     s.Intermediate.Version.String(),
				Coordinator: &idl.Segment{
					DbID:      int32(s.Intermediate.Coordinator().DbID),
					ContentID: int32(s.Intermediate.Coordinator().ContentID),
					Role:      idl.Segment_Role(idl.Segment_Role_value[s.Intermediate.Coordinator().Role]),
					Port:      int32(s.Intermediate.CoordinatorPort()),
					Hostname:  s.Intermediate.CoordinatorHostname(),
					DataDir:   s.Intermediate.CoordinatorDataDir(),
				},
			},
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
