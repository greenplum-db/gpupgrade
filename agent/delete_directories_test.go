// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestDeleteDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the data directories", func(t *testing.T) {
		var actualDirectories, actualRequiredPaths []string
		mockDeleteDirectories := func(directories []string, requiredPaths []string) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		dataDirectories := []string{
			"/data/dbfast_mirror1/seg1",
			"/data/dbfast_mirror2/seg2",
		}
		req := &idl.DeleteDataDirectoriesRequest{Datadirs: dataDirectories}

		server := agent.NewServer(agent.Config{})
		_, err := server.DeleteDataDirectories(context.Background(), req)

		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		if !reflect.DeepEqual(actualDirectories, dataDirectories) {
			t.Errorf("got directories: %s want: %s", actualDirectories, dataDirectories)
		}

		if !reflect.DeepEqual(actualRequiredPaths, upgrade.PostgresFiles) {
			t.Errorf("got required paths: %s want: %s", actualRequiredPaths, upgrade.PostgresFiles)
		}
	})
}

func TestDeleteStateDirectory(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the state directory", func(t *testing.T) {
		var actualDirectories, actualRequiredPaths []string
		mockDeleteDirectories := func(directories []string, requiredPaths []string) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		expectedDirectories := []string{"/my/state/dir"}
		server := agent.NewServer(agent.Config{
			StateDir: expectedDirectories[0],
		})

		_, err := server.DeleteStateDirectory(context.Background(), &idl.DeleteStateDirectoryRequest{})

		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		if !reflect.DeepEqual(actualDirectories, expectedDirectories) {
			t.Errorf("got directories: %s want: %s", actualDirectories, expectedDirectories)
		}

		if !reflect.DeepEqual(actualRequiredPaths, upgrade.StateDirectoryFiles) {
			t.Errorf("got required paths: %s want: %s", actualRequiredPaths, upgrade.StateDirectoryFiles)
		}
	})
}
