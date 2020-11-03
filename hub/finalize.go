// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) Finalize(_ *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	st, err := step.Begin(s.StateDir, idl.Step_FINALIZE, stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("finalize: %s", err))
		}
	}()

	st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Stop(streams)

		if err != nil {
			return xerrors.Errorf("failed to stop target cluster: %w", err)
		}

		return nil
	})

	st.Run(idl.Substep_UPDATE_TARGET_CATALOG_AND_CLUSTER_CONFIG, func(streams step.OutStreams) error {
		return s.UpdateCatalogAndClusterConfig(streams)
	})

	st.Run(idl.Substep_UPDATE_DATA_DIRECTORIES, func(_ step.OutStreams) error {
		return s.UpdateDataDirectories()
	})

	st.Run(idl.Substep_UPDATE_TARGET_CONF_FILES, func(streams step.OutStreams) error {
		return UpdateConfFiles(streams,
			s.Target.MasterDataDir(),
			s.TargetInitializeConfig.Master.Port,
			s.Source.MasterPort(),
		)
	})

	st.Run(idl.Substep_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Start(streams)

		if err != nil {
			return xerrors.Errorf("failed to start target cluster: %w", err)
		}

		return nil
	})

	// todo: we don't currently have a way to output nothing to the UI when there is no standby.
	// If we did, this check would actually be in `UpgradeStandby`
	if s.Source.HasStandby() {
		st.Run(idl.Substep_UPGRADE_STANDBY, func(streams step.OutStreams) error {
			// TODO: once the temporary standby upgrade is fixed, switch to
			// using the TargetInitializeConfig's temporary assignments, and
			// move this upgrade step back to before the target shutdown.
			standby := s.Source.Mirrors[-1]
			return UpgradeStandby(greenplum.NewRunner(s.Target, streams), StandbyConfig{
				Port:            standby.Port,
				Hostname:        standby.Hostname,
				DataDirectory:   standby.DataDir,
				UseHbaHostnames: s.UseHbaHostnames,
			})
		})
	}

	// todo: we don't currently have a way to output nothing to the UI when there are no mirrors.
	// If we did, this check would actually be in `UpgradeMirrors`
	if s.Source.HasMirrors() {
		st.Run(idl.Substep_UPGRADE_MIRRORS, func(streams step.OutStreams) error {
			// TODO: once the temporary mirror upgrade is fixed, switch to using
			// the TargetInitializeConfig's temporary assignments, and move this
			// upgrade step back to before the target shutdown.
			mirrors := func(seg *greenplum.SegConfig) bool {
				return seg.IsMirror()
			}

			return UpgradeMirrors(s.StateDir, s.Target.MasterPort(),
				s.Source.SelectSegments(mirrors), greenplum.NewRunner(s.Target, streams), s.UseHbaHostnames)
		})
	}

	// FIXME: archiveDir is not set unless we actually run this substep; it must be persisted.
	var archiveDir string
	st.Run(idl.Substep_ARCHIVE_LOG_DIRECTORIES, func(_ step.OutStreams) error {
		// Archive log directory on master
		oldDir, err := utils.GetLogDir()
		if err != nil {
			return err
		}
		archiveDir = filepath.Join(filepath.Dir(oldDir), upgrade.GetArchiveDirectoryName(s.UpgradeID, time.Now()))

		gplog.Debug("moving directory %q to %q", oldDir, archiveDir)
		if err = utils.Move(oldDir, archiveDir); err != nil {
			return err
		}

		return ArchiveSegmentLogDirectories(s.agentConns, s.Config.Target.MasterHostname(), archiveDir)
	})

	st.Run(idl.Substep_DELETE_SEGMENT_STATEDIRS, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.MasterHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_FinalizeResponse{
		FinalizeResponse: &idl.FinalizeResponse{
			TargetVersion: s.Target.Version.VersionString,
			LogArchiveDirectory: archiveDir,
			ArchivedSourceMasterDataDirectory: s.Config.TargetInitializeConfig.Master.DataDir + upgrade.OldSuffix,
			UpgradeID: s.Config.UpgradeID.String(),
			Target: &idl.Cluster{
				Port:                int32(s.Target.MasterPort()),
				MasterDataDirectory: s.Target.MasterDataDir(),
			},
		},
	}}}}

	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
