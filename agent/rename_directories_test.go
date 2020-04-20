// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestRenameDirectories(t *testing.T) {
	testhelper.SetupTestLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		dirs := []*idl.RenameDirectories{
			{Source: "", Archive: "" + upgrade.OldSuffix},
		}

		expected := errors.New("permission denied")
		agent.RenameDataDirectory = func(source, archive, target string, renameTarget bool) error {
			return expected
		}

		_, err := server.RenameDirectories(context.Background(), &idl.RenameDirectoriesRequest{Dirs: dirs})
		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}
		for _, err := range merr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("returned error %#v, want %#v", err, expected)
			}
		}
	})
}
