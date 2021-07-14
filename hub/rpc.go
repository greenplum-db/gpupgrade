// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"sync"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func ExecuteRPC(agentConns []*idl.Connection, executeRequest func(conn *idl.Connection) error) error {
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

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
