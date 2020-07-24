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
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestDeleteDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the data directories", func(t *testing.T) {
		var actualDirectories, actualRequiredPaths []string
		var actualStreams step.OutStreams
		mockDeleteDirectories := func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			actualStreams = streams
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		utils.System.Hostname = func() (string, error) {
			return "localhost.remote", nil
		}

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

		if actualStreams != step.DevNullStream {
			t.Errorf("got streams %#v want %#v", actualStreams, step.DevNullStream)
		}
	})
}

func TestDeleteStateDirectory(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the state directory", func(t *testing.T) {
		var actualDirectories, actualRequiredPaths []string
		var actualStreams step.OutStreams
		mockDeleteDirectories := func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			actualStreams = streams
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		utils.System.Hostname = func() (string, error) {
			return "localhost.remote", nil
		}

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

		if len(actualRequiredPaths) != 0 {
			t.Errorf("unexpected required paths: %s", actualRequiredPaths)
		}

		if actualStreams != step.DevNullStream {
			t.Errorf("got streams %#v want %#v", actualStreams, step.DevNullStream)
		}
	})
}
