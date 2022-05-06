// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestRenameDirectories(t *testing.T) {
	testlog.SetupLogger()
	server := agent.NewServer(agent.Config{})

	t.Run("bubbles up errors", func(t *testing.T) {
		expected := errors.New("permission denied")
		agent.RenameDirectories = func(source, target string) error {
			return expected
		}

		_, err := server.RenameDirectories(context.Background(), &idl.RenameDirectoriesRequest{Dirs: []*idl.RenameDirectories{{}}})

		if !errors.Is(err, expected) {
			t.Errorf("returned error %#v, want %#v", err, expected)
		}
	})
}
