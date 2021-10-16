// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var ErrMissingMirrorsAndStandby = errors.New("Source cluster does not have mirrors and/or standby. Cannot restore source cluster. Please contact support.")

func (s *Server) Revert(_ *idl.RevertRequest, stream idl.CliToHub_RevertServer) (err error) {
	st, err := step.Begin(idl.Step_REVERT, stream, s.AgentConns)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = errorlist.Append(err, ferr)
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("revert: %s", err))
		}
	}()

	if !s.Source.HasAllMirrorsAndStandby() {
		return errors.New("Source cluster does not have mirrors and/or standby. Cannot restore source cluster. Please contact support.")
	}

	// If the intermediate target cluster is started, it must be stopped.
	if s.Intermediate != nil {
		st.AlwaysRun(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
			running, err := s.Intermediate.IsMasterRunning(streams)
			if err != nil {
				return err
			}

			if !running {
				return step.Skip
			}

			return s.Intermediate.Stop(streams)
		})
	}

	st.RunConditionally(idl.Substep_DELETE_TARGET_CLUSTER_DATADIRS,
		s.Intermediate.Primaries != nil && s.Intermediate.MasterDataDir() != "",
		func(streams step.OutStreams) error {
			return DeleteMasterAndPrimaryDataDirectories(streams, s.agentConns, s.Intermediate)
		})

	st.RunConditionally(idl.Substep_DELETE_TABLESPACES,
		s.Intermediate.Primaries != nil && s.Intermediate.MasterDataDir() != "",
		func(streams step.OutStreams) error {
			return DeleteTargetTablespaces(streams, s.agentConns, s.Config.Intermediate, s.Intermediate.CatalogVersion, s.Source.Tablespaces)
		})

	// For any of the link-mode cases described in the "Reverting to old
	// cluster" section of https://www.postgresql.org/docs/9.4/pgupgrade.html,
	// it is correct to restore the pg_control file. Even in the case where
	// we're going to perform a full rsync restoration, we rely on this
	// substep to clean up the pg_control.old file, since the rsync will not
	// remove it.
	st.RunConditionally(idl.Substep_RESTORE_PGCONTROL, s.UseLinkMode, func(streams step.OutStreams) error {
		return RestoreMasterAndPrimariesPgControl(streams, s.agentConns, s.Source)
	})

	// if the target cluster has been started at any point, we must restore the source
	// cluster as its files could have been modified.
	targetStarted, err := step.HasRun(idl.Step_EXECUTE, idl.Substep_START_TARGET_CLUSTER)
	if err != nil {
		return err
	}

	st.RunConditionally(idl.Substep_RESTORE_SOURCE_CLUSTER, s.UseLinkMode && targetStarted, func(stream step.OutStreams) error {
		if err := RsyncMasterAndPrimaries(stream, s.agentConns, s.Source); err != nil {
			return err
		}

		return RsyncMasterAndPrimariesTablespaces(stream, s.agentConns, s.Source)
	})

	handleMirrorStartupFailure, err := s.expectMirrorFailure()
	if err != nil {
		return err
	}

	sourceClusterIsRunning, err := s.Source.IsMasterRunning(step.DevNullStream)
	if err != nil {
		return err
	}

	st.RunConditionally(idl.Substep_START_SOURCE_CLUSTER, !sourceClusterIsRunning, func(streams step.OutStreams) error {
		err = s.Source.Start(streams)
		var exitErr *exec.ExitError
		if xerrors.As(err, &exitErr) {
			// In copy mode the gpdb 5x source cluster mirrors do not come
			// up causing gpstart to return a non-zero exit status.
			// Ignore such failures, as gprecoverseg executed later will bring
			// the mirrors up
			// TODO: For 5X investigate how to check for this case and not
			//  ignore all errors with exit code 1.
			if handleMirrorStartupFailure && exitErr.ExitCode() == 1 {
				return nil
			}
		}

		if err != nil {
			return err
		}

		return nil
	})

	// Restoring the mirrors is needed in copy mode on 5X since the source cluster
	// is left in a bad state after execute. This is because running pg_upgrade on
	// a primary results in a checkpoint that does not get replicated on the mirror.
	// Thus, when the mirror is started it panics and a gprecoverseg or rsync is needed.
	st.RunConditionally(idl.Substep_RECOVERSEG_SOURCE_CLUSTER, handleMirrorStartupFailure && s.Source.Version.Major == 5, func(streams step.OutStreams) error {
		return Recoverseg(streams, s.Source, s.UseHbaHostnames)
	})

	var logArchiveDir string
	st.Run(idl.Substep_ARCHIVE_LOG_DIRECTORIES, func(_ step.OutStreams) error {
		logArchiveDir, err = s.GetLogArchiveDir()
		if err != nil {
			return xerrors.Errorf("get log archive directory: %w", err)
		}

		return ArchiveLogDirectories(logArchiveDir, s.agentConns, s.Config.Source.MasterHostname())
	})

	st.Run(idl.Substep_DELETE_SEGMENT_STATEDIRS, func(_ step.OutStreams) error {
		return DeleteStateDirectories(s.agentConns, s.Source.MasterHostname())
	})

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Contents: &idl.Response_RevertResponse{
		RevertResponse: &idl.RevertResponse{
			SourceVersion:       s.Source.Version.String(),
			LogArchiveDirectory: logArchiveDir,
			Source: &idl.Cluster{
				Port:                int32(s.Source.MasterPort()),
				MasterDataDirectory: s.Source.MasterDataDir(),
			},
		},
	}}}}

	if err := stream.Send(message); err != nil {
		return xerrors.Errorf("sending response message: %w", err)
	}

	return st.Err()
}

// In 5X, running pg_upgrade on the primaries can cause the mirrors to receive an invalid
// checkpoint upon starting. There are two ways to resolve this:
// - Rsync from the corresponding mirrors
// or
// - Running recoverseg
// If the former hasn't run yet, then we do expect mirror failure upon start, so return true.
func (s *Server) expectMirrorFailure() (bool, error) {
	// mirror startup failure is expected only for GPDB 5x
	if s.Source.Version.Major != 5 {
		return false, nil
	}

	hasRestoreRun, err := step.HasRun(idl.Step_REVERT, idl.Substep_RESTORE_SOURCE_CLUSTER)
	if err != nil {
		return false, err
	}

	primariesUpgraded, err := step.HasRun(idl.Step_EXECUTE, idl.Substep_UPGRADE_PRIMARIES)
	if err != nil {
		return false, err
	}

	return !hasRestoreRun && primariesUpgraded, nil
}
