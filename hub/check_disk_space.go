// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"sync"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/disk"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var checkDiskUsage = disk.CheckUsage

func CheckDiskSpace(streams step.OutStreams, agentConns []*Connection, diskFreeRatio float64, source *greenplum.Cluster, sourceTablespaces greenplum.Tablespaces) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(agentConns)+1)
	usages := make(chan disk.FileSystemDiskUsage, len(agentConns)+1)

	// check disk space on master
	wg.Add(1)
	go func() {
		defer wg.Done()

		masterDirs := []string{source.MasterDataDir()}
		masterDirs = append(masterDirs, sourceTablespaces.GetMasterTablespaces().UserDefinedTablespacesLocations()...)

		usage, err := checkDiskUsage(streams, disk.Local, diskFreeRatio, masterDirs...)
		errs <- err
		usages <- usage
	}()

	checkDiskSpaceOnStandbyAndSegments(agentConns, errs, usages, diskFreeRatio, source, sourceTablespaces)

	wg.Wait()
	close(errs)
	close(usages)

	// consolidate errors
	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	if err != nil {
		return err
	}

	// combine disk space usage across all hosts and return an usage error
	var totalUsage disk.FileSystemDiskUsage
	for usage := range usages {
		totalUsage = append(totalUsage, usage...)
	}

	if totalUsage != nil {
		return disk.NewSpaceUsageError(totalUsage)
	}

	return nil
}

func checkDiskSpaceOnStandbyAndSegments(agentConns []*Connection, errs chan<- error, usages chan<- disk.FileSystemDiskUsage, diskFreeRatio float64, source *greenplum.Cluster, sourceTablespaces greenplum.Tablespaces) {
	var wg sync.WaitGroup

	for _, conn := range agentConns {
		conn := conn

		segmentsExcludingMaster := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsMaster()
		})

		if len(segmentsExcludingMaster) == 0 {
			return
		}

		var dirs []string
		for _, seg := range segmentsExcludingMaster {
			dirs = append(dirs, seg.DataDir)
			dirs = append(dirs, sourceTablespaces[seg.DbID].UserDefinedTablespacesLocations()...)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			req := &idl.CheckSegmentDiskSpaceRequest{
				DiskFreeRatio: diskFreeRatio,
				Dirs:          dirs,
			}

			reply, err := conn.AgentClient.CheckDiskSpace(context.Background(), req)
			errs <- err
			if reply != nil {
				usages <- reply.GetUsage()
			}
		}()
	}

	wg.Wait()
}
