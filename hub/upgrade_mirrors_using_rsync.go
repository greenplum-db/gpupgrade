// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func UpgradeMirrorsUsingRsync(conn *greenplum.Conn, agentConns []*idl.Connection, source *greenplum.Cluster, intermediate *greenplum.Cluster, useHbaHostnames bool) error {
	options := []greenplum.Option{
		greenplum.ToTarget(),
		greenplum.Port(intermediate.MasterPort()),
	}

	db, err := sql.Open("pgx", conn.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	if err := CreateReplicationSlots(db); err != nil {
		return err
	}

	if err := intermediate.Stop(step.DevNullStream); err != nil {
		return err
	}

	if err := RsyncMirrorDataDirsOnSegments(agentConns, source, intermediate); err != nil {
		return err
	}

	if err := RsyncMirrorTablespacesOnSegments(agentConns, source, intermediate); err != nil {
		return err
	}

	if err := RenameMirrorTablespacesOnSegments(agentConns, source, intermediate); err != nil {
		return err
	}

	if err := CreateRecoveryConfOnSegments(agentConns, intermediate); err != nil {
		return err
	}

	if err := AddReplicationEntriesOnPrimaries(agentConns, intermediate, useHbaHostnames); err != nil {
		return err
	}

	if err := UpdateInternalAutoConfOnMirrors(agentConns, intermediate); err != nil {
		return err
	}

	if err := intermediate.StartMasterOnly(step.DevNullStream); err != nil {
		return err
	}

	if err := addMirrorsToCatalog(conn, intermediate); err != nil {
		return err
	}

	if err := intermediate.StopMasterOnly(step.DevNullStream); err != nil {
		return err
	}

	if err := intermediate.Start(step.DevNullStream); err != nil {
		return err
	}

	return nil
}

func RsyncMirrorDataDirsOnSegments(agentConns []*idl.Connection, source *greenplum.Cluster, intermediate *greenplum.Cluster) error {
	request := func(conn *idl.Connection) error {
		sourcePrimaries := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsMaster() && seg.IsPrimary()
		})

		var opts []*idl.RsyncRequest_RsyncOptions
		for _, sourcePrimary := range sourcePrimaries {
			intermediatePrimary := intermediate.Primaries[sourcePrimary.ContentID]
			intermediateMirror := intermediate.Mirrors[sourcePrimary.ContentID]

			// On the source primary host rsync to the intermediate mirror host
			// copy both the source & intermediate primary data directories to the intermediate mirror data directory.
			opt := &idl.RsyncRequest_RsyncOptions{
				Sources:         []string{sourcePrimary.DataDir, intermediatePrimary.DataDir},
				Destination:     filepath.Dir(intermediateMirror.DataDir), // FIXME: Do we really want filepath.Dir here
				DestinationHost: intermediateMirror.Hostname,
				Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
			}

			opts = append(opts, opt)
		}

		req := &idl.RsyncRequest{Options: opts}
		_, err := conn.AgentClient.RsyncDataDirectories(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func RsyncMirrorTablespacesOnSegments(agentConns []*idl.Connection, source *greenplum.Cluster, intermediate *greenplum.Cluster) error {
	request := func(conn *idl.Connection) error {
		sourcePrimaries := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsMaster() && seg.IsPrimary()
		})

		var opts []*idl.RsyncRequest_RsyncOptions
		for _, sourcePrimary := range sourcePrimaries {
			intermediateMirror := intermediate.Mirrors[sourcePrimary.ContentID]

			for tsOid, sourcePrimaryTsInfo := range source.Tablespaces[sourcePrimary.DbID] {
				if !sourcePrimaryTsInfo.IsUserDefined() {
					continue
				}

				sourcePrimaryTsLocation := sourcePrimaryTsInfo.Location + string(os.PathSeparator)
				sourceMirrorTsLocation := source.Tablespaces[intermediateMirror.DbID][tsOid].Location

				// On the source primary host rsync to the intermediate mirror host the source primary tablespaces.
				opt := &idl.RsyncRequest_RsyncOptions{
					Sources:         []string{sourcePrimaryTsLocation},
					Destination:     sourceMirrorTsLocation,
					DestinationHost: intermediateMirror.Hostname,
					Options:         []string{"--archive", "--delete", "--hard-links", "--size-only", "--no-inc-recursive"},
				}

				opts = append(opts, opt)
			}
		}

		_, err := conn.AgentClient.RsyncTablespaceDirectories(context.Background(), &idl.RsyncRequest{Options: opts})
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func RenameMirrorTablespacesOnSegments(agentConns []*idl.Connection, source *greenplum.Cluster, intermediate *greenplum.Cluster) error {
	request := func(conn *idl.Connection) error {
		intermediateMirrors := intermediate.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsStandby() && seg.IsMirror()
		})

		var pairs []*idl.RenameTablespacesRequest_RenamePair
		for _, intermediateMirror := range intermediateMirrors {
			intermediatePrimary := intermediate.Primaries[intermediateMirror.ContentID]
			sourcePrimary := source.Primaries[intermediateMirror.ContentID]

			for tsOid, sourcePrimaryTsInfo := range source.Tablespaces[sourcePrimary.DbID] {
				if !sourcePrimaryTsInfo.IsUserDefined() {
					continue
				}

				sourceMirrorTsLocation := source.Tablespaces[intermediateMirror.DbID][tsOid].Location
				sourcePrimaryTsLocation := sourcePrimaryTsInfo.Location

				// Since we bootstrapped the mirror tablespaces by coping the primary tablespaces we need to fix the
				// directory names by renaming the primary DbID to the mirror DbID. We do this on the host containing
				// the mirror tablespaces.
				pair := &idl.RenameTablespacesRequest_RenamePair{
					Source:      filepath.Join(sourceMirrorTsLocation, strconv.Itoa(intermediatePrimary.DbID)),
					Destination: filepath.Join(sourcePrimaryTsLocation, strconv.Itoa(intermediateMirror.DbID)),
				}

				pairs = append(pairs, pair)
			}
		}

		_, err := conn.AgentClient.RenameTablespaces(context.Background(), &idl.RenameTablespacesRequest{RenamePairs: pairs})
		return err
	}

	return ExecuteRPC(agentConns, request)
}
