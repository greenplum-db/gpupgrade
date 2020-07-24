// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) Revert(_ *idl.RevertRequest, stream idl.CliToHub_RevertServer) (err error) {
	st, err := step.Begin(s.StateDir, idl.Step_REVERT, stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("revert: %s", err))
		}
	}()

	if !s.Source.HasAllMirrorsAndStandby() {
		return errors.New("Source cluster does not have mirrors and/or standby. Cannot restore source cluster. Please contact support.")
	}

	// Since revert needs to work at any point, and stop is not yet idempotent
	// check if the cluster is running before stopping.
	// TODO: This will fail if the target does not exist which can occur when
	//  initialize fails part way through and does not create the target cluster.
	running, err := s.Target.IsMasterRunning(st.Streams())
	if err != nil {
		return err
	}

	if running {
		st.Run(idl.Substep_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
			if err := s.Target.Stop(streams); err != nil {
				return xerrors.Errorf("stopping target cluster: %w", err)
			}
			return nil
		})
	}

	// Restoring the source master and primaries is only needed if upgrading the
	// primaries had started.
	// TODO: For now we use if the source master is not running to determine this.
	running, err = s.Source.IsMasterRunning(st.Streams())
	if err != nil {
		return err
	}

	if !running && s.UseLinkMode {
		st.Run(idl.Substep_RESTORE_SOURCE_CLUSTER, func(stream step.OutStreams) error {
			return RsyncMasterAndPrimaries(stream, s.agentConns, s.Source)
		})
	}

	if len(s.Config.Target.Primaries) > 0 {
		st.Run(idl.Substep_DELETE_PRIMARY_DATADIRS, func(_ step.OutStreams) error {
			return DeletePrimaryDataDirectories(s.agentConns, s.Config.Target)
		})

		st.Run(idl.Substep_DELETE_MASTER_DATADIR, func(streams step.OutStreams) error {
			datadir := s.Config.Target.MasterDataDir()
			return upgrade.DeleteDirectories([]string{datadir}, upgrade.PostgresFiles, streams)
		})
	}

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

	// Since revert needs to work at any point, and start is not yet idempotent
	// check if the cluster is not running before starting.
	running, err = s.Source.IsMasterRunning(st.Streams())
	if err != nil {
		return err
	}

	if !running {
		st.Run(idl.Substep_START_SOURCE_CLUSTER, func(streams step.OutStreams) error {
			err := s.Source.Start(streams)
			var exitErr *exec.ExitError
			if xerrors.As(err, &exitErr) {
				// In copy mode the gpdb 5x source cluster mirrors do not come
				// up causing gpstart to return a non-zero exit status.
				// This substep fails preventing the following substep steps
				// from running including gprecoverseg.
				// TODO: For 5X investigate how to check for this case and not
				//  ignore all errors with exit code 1.
				if !s.UseLinkMode && exitErr.ExitCode() == 1 {
					return nil
				}
			}

			if err != nil {
				return xerrors.Errorf("starting source cluster: %w", err)
			}

			return nil
		})
	}

	if !s.UseLinkMode {
		st.Run(idl.Substep_RESTORE_SOURCE_CLUSTER, func(streams step.OutStreams) error {
			return Recoverseg(streams, s.Source)
		})
	}

	message := &idl.Message{Contents: &idl.Message_Response{Response: &idl.Response{Data: map[string]string{
		idl.ResponseKey_source_version.String():               s.Source.Version.VersionString,
		idl.ResponseKey_revert_log_archive_directory.String(): archiveDir,
	}}}}
	if err := stream.Send(message); err != nil {
		return xerrors.Errorf("sending response message: %w", err)
	}

	return st.Err()
}
