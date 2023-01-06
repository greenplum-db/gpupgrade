// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"sort"
	"sync"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/disk"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var checkDiskUsage = disk.CheckUsage

func CheckDiskSpace(streams step.OutStreams, agentConns []*idl.Connection, diskFreeRatio float64, source *greenplum.Cluster, sourceTablespaces greenplum.Tablespaces) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(agentConns)+1)
	usagesChan := make(chan disk.FileSystemDiskUsage, len(agentConns)+1)

	// check disk space on coordinator
	wg.Add(1)
	go func() {
		defer wg.Done()

		coordinatorDirs := []string{source.CoordinatorDataDir()}
		coordinatorDirs = append(coordinatorDirs, sourceTablespaces.GetCoordinatorTablespaces().UserDefinedTablespacesLocations()...)

		usage, err := checkDiskUsage(streams, disk.Local, diskFreeRatio, coordinatorDirs...)
		errs <- err
		usagesChan <- usage
	}()

	checkDiskSpaceOnStandbyAndSegments(agentConns, errs, usagesChan, diskFreeRatio, source, sourceTablespaces)

	wg.Wait()
	close(errs)
	close(usagesChan)

	// consolidate errors
	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	if err != nil {
		return err
	}

	// combine disk space usage across all hosts and return an usage error
	totalUsage := make(map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage)
	for usages := range usagesChan {
		for _, usage := range usages {
			totalUsage[disk.FilesystemHost{Filesystem: usage.GetFs(), Host: usage.GetHost()}] = usage
		}
	}

	if len(totalUsage) > 0 {
		return disk.NewSpaceUsageError(totalUsage)
	}

	return nil
}

func checkDiskSpaceOnStandbyAndSegments(agentConns []*idl.Connection, errs chan<- error, usages chan<- disk.FileSystemDiskUsage, diskFreeRatio float64, source *greenplum.Cluster, sourceTablespaces greenplum.Tablespaces) {
	var wg sync.WaitGroup

	for _, conn := range agentConns {
		conn := conn

		segmentsExcludingCoordinator := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsCoordinator()
		})
		sort.Sort(segmentsExcludingCoordinator)
		if len(segmentsExcludingCoordinator) == 0 {
			return
		}

		var dirs []string
		for _, seg := range segmentsExcludingCoordinator {
			dirs = append(dirs, seg.DataDir)
			dirs = append(dirs, sourceTablespaces[int32(seg.DbID)].UserDefinedTablespacesLocations()...)
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
