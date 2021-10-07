// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) CreateRecoveryConf(ctx context.Context, req *idl.CreateRecoveryConfRequest) (*idl.CreateRecoveryConfReply, error) {
	gplog.Info("agent received request to create recovery.conf")

	err := createRecoveryConf(req.GetConnections())
	if err != nil {
		return &idl.CreateRecoveryConfReply{}, err
	}

	return &idl.CreateRecoveryConfReply{}, nil
}

func createRecoveryConf(connReqs []*idl.CreateRecoveryConfRequest_Connection) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(connReqs))

	for _, connReq := range connReqs {
		wg.Add(1)

		go func(connReq *idl.CreateRecoveryConfRequest_Connection) {
			defer wg.Done()

			config := fmt.Sprintf(`standby_mode = 'on'
primary_conninfo = 'user=%s host=%s port=%d sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'`, connReq.GetUser(), connReq.GetPrimaryHost(), connReq.GetPrimaryPort())

			err := os.WriteFile(filepath.Join(connReq.GetMirrorDataDir(), "recovery.conf"), []byte(config), 0644)
			if err != nil {
				errs <- err
			}
		}(connReq)
	}

	wg.Wait()
	close(errs)

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
