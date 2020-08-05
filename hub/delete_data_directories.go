// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"sync"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/context"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func DeleteMirrorAndStandbyDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	segs := cluster.SelectSegments(func(seg *greenplum.SegConfig) bool {
		return seg.Role == greenplum.MirrorRole
	})
	return deleteDataDirectories(agentConns, segs)
}

func DeletePrimaryDataDirectories(agentConns []*Connection, segConfigs greenplum.SegConfigs) error {
	return deleteDataDirectories(agentConns, segConfigs)
}

func deleteDataDirectories(agentConns []*Connection, segConfigs greenplum.SegConfigs) error {
	request := func(conn *Connection) error {

		segs := segConfigs.Select(func(seg *greenplum.SegConfig) bool {
			return seg.Hostname == conn.Hostname
		})

		if len(segs) == 0 {
			// This can happen if there are no segments matching the filter on a host
			return nil
		}

		req := new(idl.DeleteDataDirectoriesRequest)
		for _, seg := range segs {
			datadir := seg.DataDir
			req.Datadirs = append(req.Datadirs, datadir)
		}

		_, err := conn.AgentClient.DeleteDataDirectories(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func DeleteTargetTablespaces(streams step.OutStreams, agentConns []*Connection, target *greenplum.Cluster, targetCatalogVersion string, sourceTablespaces greenplum.Tablespaces) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- DeleteTargetTablespacesOnMaster(streams, target, sourceTablespaces.GetMasterTablespaces(), targetCatalogVersion)
	}()

	errs <- DeleteTargetTablespacesOnPrimaries(agentConns, target, sourceTablespaces, targetCatalogVersion)

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

func DeleteTargetTablespacesOnMaster(streams step.OutStreams, target *greenplum.Cluster, masterTablespaces greenplum.SegmentTablespaces, catalogVersion string) error {
	var dirs []string
	for _, tsInfo := range masterTablespaces {
		if !tsInfo.IsUserDefined() {
			continue
		}

		path := upgrade.TablespacePath(tsInfo.Location, target.Master().DbID, target.Version.SemVer.Major, catalogVersion)
		dirs = append(dirs, path)
	}

	return upgrade.DeleteNewTablespaceDirectories(streams, dirs)
}

func DeleteTargetTablespacesOnPrimaries(agentConns []*Connection, target *greenplum.Cluster, tablespaces greenplum.Tablespaces, catalogVersion string) error {
	request := func(conn *Connection) error {
		primaries := target.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && seg.IsPrimary() && !seg.IsMaster()
		})

		if len(primaries) == 0 {
			return nil
		}

		var dirs []string
		for _, seg := range primaries {
			segTablespaces := tablespaces[seg.DbID]
			for _, tsInfo := range segTablespaces {
				if !tsInfo.IsUserDefined() {
					continue
				}

				path := upgrade.TablespacePath(tsInfo.Location, seg.DbID, target.Version.SemVer.Major, catalogVersion)
				dirs = append(dirs, path)
			}
		}

		req := &idl.DeleteTablespaceRequest{Dirs: dirs}
		_, err := conn.AgentClient.DeleteTablespaceDirectories(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}
