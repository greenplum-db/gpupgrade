// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestDeleteDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes data directories", func(t *testing.T) {
		utils.System.Hostname = func() (string, error) {
			return "localhost.remote", nil
		}

		dirs := []string{"/data/dbfast_mirror1/seg1", "/data/dbfast_mirror2/seg2"}
		agent.DeleteDirectoriesFunc = func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			if !reflect.DeepEqual(directories, dirs) {
				t.Errorf("got directories %q want %q", directories, dirs)
			}

			if !reflect.DeepEqual(requiredPaths, upgrade.PostgresFiles) {
				t.Errorf("got required paths %q wantq %q", requiredPaths, upgrade.PostgresFiles)
			}

			if streams != step.DevNullStream {
				t.Errorf("got streams %#v want %#v", streams, step.DevNullStream)
			}

			return nil
		}

		server := agent.NewServer(agent.Config{})
		req := &idl.DeleteDataDirectoriesRequest{Datadirs: dirs}
		_, err := server.DeleteDataDirectories(context.Background(), req)
		if err != nil {
			t.Errorf("DeleteDataDirectories returned error %+v", err)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		expected := errors.New("error")
		agent.DeleteDirectoriesFunc = func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			return expected
		}

		server := agent.NewServer(agent.Config{})
		req := &idl.DeleteDataDirectoriesRequest{}
		_, err := server.DeleteDataDirectories(context.Background(), req)
		if !xerrors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", expected, err)
		}
	})
}

func TestDeleteStateDirectory(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("deletes the state directory", func(t *testing.T) {
		utils.System.Hostname = func() (string, error) {
			return "localhost.remote", nil
		}

		dir := []string{"/my/state/dir"}
		agent.DeleteDirectoriesFunc = func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			if !reflect.DeepEqual(directories, dir) {
				t.Errorf("got directories %q want %q", directories, dir)
			}

			if len(requiredPaths) != 0 {
				t.Errorf("expected no required paths but found %q", requiredPaths)
			}

			if streams != step.DevNullStream {
				t.Errorf("got streams %#v want %#v", streams, step.DevNullStream)
			}

			return nil
		}

		server := agent.NewServer(agent.Config{StateDir: dir[0]})
		req := &idl.DeleteStateDirectoryRequest{}
		_, err := server.DeleteStateDirectory(context.Background(), req)
		if err != nil {
			t.Errorf("DeleteStateDirectory returned error %+v", err)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		expected := errors.New("error")
		agent.DeleteDirectoriesFunc = func(directories []string, requiredPaths []string, streams step.OutStreams) error {
			return expected
		}

		server := agent.NewServer(agent.Config{})
		req := &idl.DeleteStateDirectoryRequest{}
		_, err := server.DeleteStateDirectory(context.Background(), req)
		if !xerrors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", expected, err)
		}
	})
}
