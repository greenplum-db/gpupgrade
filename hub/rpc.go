// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"sync"

	"github.com/hashicorp/go-multierror"
)

func ExecuteRPC(agentConns []*Connection, executeRequest func(conn *Connection) error) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		wg.Add(1)
		go func() {
			defer wg.Done()

			err := executeRequest(conn)
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	var mErr *multierror.Error
	for err := range errs {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}
