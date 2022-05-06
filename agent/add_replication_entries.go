// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func (s *Server) AddReplicationEntries(ctx context.Context, req *idl.AddReplicationEntriesRequest) (*idl.AddReplicationEntriesReply, error) {
	gplog.Info("agent received request to add replication entries to pg_hba.conf")

	err := AddReplicationEntriesToPgHbaConf(req.GetEntries())
	if err != nil {
		return &idl.AddReplicationEntriesReply{}, err
	}

	return &idl.AddReplicationEntriesReply{}, nil
}

func AddReplicationEntriesToPgHbaConf(confs []*idl.AddReplicationEntriesRequest_Entry) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(confs))

	for _, conf := range confs {

		wg.Add(1)
		go func(conf *idl.AddReplicationEntriesRequest_Entry) {
			defer wg.Done()

			var lines strings.Builder
			lines.WriteString(fmt.Sprintf("host replication %s samehost trust\n", conf.GetUser()))
			for _, mirrorHostAddr := range conf.GetHostAddrs() {
				lines.WriteString(fmt.Sprintf("host all %s %s trust\n", conf.GetUser(), mirrorHostAddr))
				lines.WriteString(fmt.Sprintf("host replication %s %s trust\n", conf.GetUser(), mirrorHostAddr))
			}

			file, err := os.OpenFile(filepath.Join(conf.GetDataDir(), "pg_hba.conf"), os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				errs <- err
				return
			}
			defer func() {
				if cErr := file.Close(); cErr != nil {
					errs <- errorlist.Append(err, cErr)
				}
			}()

			_, err = file.WriteString(lines.String())
			if err != nil {
				errs <- err
			}
		}(conf)
	}

	wg.Wait()
	close(errs)

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
