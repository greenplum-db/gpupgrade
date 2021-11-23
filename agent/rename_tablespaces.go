//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

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

func (s *Server) RenameTablespaces(ctx context.Context, req *idl.RenameTablespacesRequest) (*idl.RenameTablespacesReply, error) {
	gplog.Info("agent received request to rename tablespaces")

	err := renameTablespaces(req.GetRenamePairs())
	if err != nil {
		return &idl.RenameTablespacesReply{}, err
	}

	return &idl.RenameTablespacesReply{}, nil
}

func renameTablespaces(pairs []*idl.RenameTablespacesRequest_RenamePair) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(pairs)*2)

	for _, pair := range pairs {
		wg.Add(1)

		go func(pair *idl.RenameTablespacesRequest_RenamePair) {
			defer wg.Done()

			gplog.Info("mkdirAll: %q", filepath.Dir(pair.GetDestination()))
			err := os.MkdirAll(filepath.Dir(pair.GetDestination()), 0700)
			if err != nil {
				errs <- fmt.Errorf("on host %q: %w", hostname, err)
			}

			gplog.Info("rename: %q to %q", pair.GetSource(), pair.GetDestination())
			err = os.Rename(pair.GetSource(), pair.GetDestination())
			if err != nil {
				errs <- fmt.Errorf("on host %q: %w", hostname, err)
			}
		}(pair)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}
