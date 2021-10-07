//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestRenameTablespaces(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("succeeds", func(t *testing.T) {
		_, _, tsLocation := testutils.MustMakeTablespaceDir(t, 16386)
		defer testutils.MustRemoveAll(t, tsLocation)

		src := filepath.Join(tsLocation, "1")
		dst := filepath.Join(tsLocation, "0")
		pairs := []*idl.RenameTablespacesRequest_RenamePair{{
			Source:      src,
			Destination: dst,
		}}

		_, err := server.RenameTablespaces(context.Background(), &idl.RenameTablespacesRequest{RenamePairs: pairs})
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		testutils.PathMustExist(t, dst)
		testutils.PathMustNotExist(t, src)
	})

	t.Run("returns multiple errors when failing to rename tablespaces", func(t *testing.T) {
		dst := "/tmp/1"
		testutils.MustWriteToFile(t, dst, "")
		defer testutils.MustRemoveAll(t, dst)

		pairs := []*idl.RenameTablespacesRequest_RenamePair{
			{
				Source:      "/does/not/exist",
				Destination: "/does/not/exist/1",
			},
			{
				Source:      "/also/does/not/exist",
				Destination: "/also/does/not/exist/1",
			}}

		_, err := server.RenameTablespaces(context.Background(), &idl.RenameTablespacesRequest{RenamePairs: pairs})
		if err == nil {
			t.Error("expected error, returned nil")
		}

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 4 {
			t.Errorf("got %d errors want 2", len(errs))
		}

		for _, err := range errs {
			var pathError *os.PathError
			var linkError *os.LinkError
			if !(errors.As(err, &pathError) || errors.As(err, &linkError)) {
				t.Errorf("got type %T want %T or %T", err, pathError, linkError)
			}
		}
	})
}
