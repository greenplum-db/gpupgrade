// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
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
		var actualHostname string
		var actualStreams step.OutStreams
		mockDeleteDirectories := func(directories []string, requiredPaths []string, hostname string, streams step.OutStreams) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			actualHostname = hostname
			actualStreams = streams
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		expectedHostname := "localhost.remote"
		utils.System.Hostname = func() (s string, err error) {
			return expectedHostname, nil
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

		if actualHostname != expectedHostname {
			t.Errorf("got hostname %q want %q", actualHostname, expectedHostname)
		}

		if actualStreams != utils.DevNull {
			t.Errorf("got streams %#v want %#v", actualStreams, utils.DevNull)
		}
	})

	t.Run("errors when System.Hostname() returns an error", func(t *testing.T) {
		expectedErr := errors.New("hostname error")
		utils.System.Hostname = func() (string, error) {
			return "", expectedErr
		}

		cleanup := agent.SetDeleteDirectories(func(strings []string, strings2 []string, s string, streams step.OutStreams) error {
			return nil
		})
		defer cleanup()

		server := agent.NewServer(agent.Config{})
		_, err := server.DeleteStateDirectory(context.Background(), &idl.DeleteStateDirectoryRequest{})

		if err != expectedErr {
			t.Errorf("got error %#v want %#v", err, expectedErr)
		}
	})
}

func TestDeleteStateDirectory(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the state directory", func(t *testing.T) {
		var actualDirectories, actualRequiredPaths []string
		var actualHostname string
		var actualStreams step.OutStreams
		mockDeleteDirectories := func(directories []string, requiredPaths []string, hostname string, streams step.OutStreams) error {
			actualDirectories = directories
			actualRequiredPaths = requiredPaths
			actualHostname = hostname
			actualStreams = streams
			return nil
		}
		cleanup := agent.SetDeleteDirectories(mockDeleteDirectories)
		defer cleanup()

		expectedHostname := "localhost.remote"
		utils.System.Hostname = func() (s string, err error) {
			return expectedHostname, nil
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

		if !reflect.DeepEqual(actualRequiredPaths, upgrade.StateDirectoryFiles) {
			t.Errorf("got required paths: %s want: %s", actualRequiredPaths, upgrade.StateDirectoryFiles)
		}

		if actualHostname != expectedHostname {
			t.Errorf("got hostname %q want %q", actualHostname, expectedHostname)
		}

		if actualStreams != utils.DevNull {
			t.Errorf("got streams %#v want %#v", actualStreams, utils.DevNull)
		}
	})

	t.Run("errors when System.Hostname() returns an error", func(t *testing.T) {
		expectedErr := errors.New("hostname error")
		utils.System.Hostname = func() (string, error) {
			return "", expectedErr
		}

		cleanup := agent.SetDeleteDirectories(func(strings []string, strings2 []string, s string, streams step.OutStreams) error {
			return nil
		})
		defer cleanup()

		server := agent.NewServer(agent.Config{})
		_, err := server.DeleteStateDirectory(context.Background(), &idl.DeleteStateDirectoryRequest{})

		if err != expectedErr {
			t.Errorf("got error %#v want %#v", err, expectedErr)
		}
	})
}
