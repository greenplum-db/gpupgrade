// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestRenameDirectories(t *testing.T) {
	testlog.SetupLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		expected := errors.New("permission denied")
		agent.ArchiveSource = func(source, target string, renameTarget bool) error {
			return expected
		}

		_, err := server.RenameDirectories(context.Background(), &idl.RenameDirectoriesRequest{Dirs: []*idl.RenameDirectories{{}}})
		var merr *multierror.Error
		if !errors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}
		for _, err := range merr.Errors {
			if !errors.Is(err, expected) {
				t.Errorf("returned error %#v, want %#v", err, expected)
			}
		}
	})
}
