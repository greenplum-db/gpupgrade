// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

func (s *Server) Finalize(_ *idl.FinalizeRequest, stream idl.CliToHub_FinalizeServer) (err error) {
	st, err := step.Begin(s.StateDir, "finalize", stream)
	if err != nil {
		return err
	}

	defer func() {
		if ferr := st.Finish(); ferr != nil {
			err = multierror.Append(err, ferr).ErrorOrNil()
		}

		if err != nil {
			gplog.Error(fmt.Sprintf("finalize: %s", err))
		}
	}()

	st.Run(idl.Substep_FINALIZE_SHUTDOWN_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Stop(streams)

		if err != nil {
			return xerrors.Errorf("failed to stop target cluster: %w", err)
		}

		return nil
	})

	st.Run(idl.Substep_FINALIZE_UPDATE_TARGET_CATALOG_AND_CLUSTER_CONFIG, func(streams step.OutStreams) error {
		return s.UpdateCatalogAndClusterConfig(streams)
	})

	st.Run(idl.Substep_FINALIZE_UPDATE_DATA_DIRECTORIES, func(_ step.OutStreams) error {
		return s.UpdateDataDirectories()
	})

	st.Run(idl.Substep_FINALIZE_UPDATE_TARGET_CONF_FILES, func(streams step.OutStreams) error {
		return UpdateConfFiles(streams,
			s.Target.MasterDataDir(),
			s.TargetInitializeConfig.Master.Port,
			s.Source.MasterPort(),
		)
	})

	st.Run(idl.Substep_FINALIZE_START_TARGET_CLUSTER, func(streams step.OutStreams) error {
		err := s.Target.Start(streams)

		if err != nil {
			return xerrors.Errorf("failed to start target cluster: %w", err)
		}

		return nil
	})

	// todo: we don't currently have a way to output nothing to the UI when there is no standby.
	// If we did, this check would actually be in `UpgradeStandby`
	if s.Source.HasStandby() {
		st.Run(idl.Substep_FINALIZE_UPGRADE_STANDBY, func(streams step.OutStreams) error {
			// TODO: once the temporary standby upgrade is fixed, switch to
			// using the TargetInitializeConfig's temporary assignments, and
			// move this upgrade step back to before the target shutdown.
			standby := s.Source.Mirrors[-1]
			return UpgradeStandby(greenplum.NewRunner(s.Target, streams), StandbyConfig{
				Port:          standby.Port,
				Hostname:      standby.Hostname,
				DataDirectory: standby.DataDir,
			})
		})
	}

	// todo: we don't currently have a way to output nothing to the UI when there are no mirrors.
	// If we did, this check would actually be in `UpgradeMirrors`
	if s.Source.HasMirrors() {
		st.Run(idl.Substep_FINALIZE_UPGRADE_MIRRORS, func(streams step.OutStreams) error {
			// TODO: once the temporary mirror upgrade is fixed, switch to using
			// the TargetInitializeConfig's temporary assignments, and move this
			// upgrade step back to before the target shutdown.
			mirrors := func(seg *greenplum.SegConfig) bool {
				return seg.IsMirror()
			}

			return UpgradeMirrors(s.StateDir, s.Target.MasterPort(),
				s.Source.SelectSegments(mirrors), greenplum.NewRunner(s.Target, streams))
		})
	}

	message := MakeTargetClusterMessage(s.Target)
	if err = stream.Send(message); err != nil {
		return err
	}

	return st.Err()
}
