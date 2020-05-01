// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
)

func DeleteStateDirectories(agentConns []*Connection, excludeHostname string) error {
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

			_, err := conn.AgentClient.DeleteStateDirectory(context.Background(), &idl.DeleteStateDirectoryRequest{})
			if err != nil {
				gplog.Error("Error deleting state directory on host %s: %s",
					conn.Hostname, err.Error())
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
