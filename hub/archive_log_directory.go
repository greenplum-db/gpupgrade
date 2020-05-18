// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
)

func ArchiveSegmentLogDirectories(agentConns []*Connection, excludeHostname, newDir string) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		// Skip the state directory that is on the master host, which we delete
		// later from the hub.
		if conn.Hostname == excludeHostname {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := conn.AgentClient.ArchiveLogDirectory(context.Background(), &idl.ArchiveLogDirectoryRequest{
				NewDir: newDir,
			})
			if err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	var mErr *multierror.Error
	for err := range errChan {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
