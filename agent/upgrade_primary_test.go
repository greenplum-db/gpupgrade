// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
)

func getSampleSegment() agent.Segment {
	tablespaces := map[int32]*idl.TablespaceInfo{
		1663: {
			Name:        "default",
			Location:    "/tmp/default/1663",
			UserDefined: true,
		},
	}

	segment := agent.Segment{
		DataDirPair: &idl.DataDirPair{
			TargetDataDir: "/tmp/newprimary1",
			DBID:          2,
			Tablespaces:   tablespaces,
		},
	}

	return segment
}
func TestRestoreTablespaces(t *testing.T) {
	t.Run("successfully performs restore of tablespaces", func(t *testing.T) {
		request := &idl.UpgradePrimariesRequest{
			TablespacesMappingFilePath: "/tmp/tablespaces/tablespace_mapping.txt",
		}

		tablespaces := map[int32]*idl.TablespaceInfo{
			1663: {
				Name:        "default",
				Location:    "/tmp/default/1663",
				UserDefined: true,
			},
			1665: {
				Name:        "another",
				Location:    "/tmp/other/1665",
				UserDefined: false,
			},
		}
		segment := agent.Segment{
			DataDirPair: &idl.DataDirPair{
				TargetDataDir: "/tmp/newprimary1",
				DBID:          2,
				Tablespaces:   tablespaces,
			},
		}

		// all the args for multiple invocations of rsync
		expectedRsyncArgs := []string{
			"--archive", "--delete",
			"/tmp/tablespaces/1663/1/", "/tmp/default/1663/2",
		}

		var actualArgs []string
		agent.SetRsyncCommand(exectest.NewCommandWithVerifier(agent.Success, func(name string, args ...string) {
			expected := "rsync"
			if name != expected {
				t.Errorf("RestoreTablespaces() invoked %q, want %q", name, expected)
			}

			// will use for validation later
			actualArgs = append(actualArgs, args...)
		}))

		var actualLstatLinks []string
		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			actualLstatLinks = append(actualLstatLinks, name)

			return nil, nil
		}

		var actualRemovedLinks []string
		utils.System.Remove = func(name string) error {
			actualRemovedLinks = append(actualRemovedLinks, name)
			return nil
		}

		var actualNewSymLink []string
		var actualOldName []string
		utils.System.Symlink = func(oldname, newname string) error {
			actualNewSymLink = append(actualNewSymLink, newname)
			actualOldName = append(actualOldName, oldname)

			return nil
		}

		defer func() { agent.SetRsyncCommand(nil) }()

		expectedRecreateSymLinkArgs := [][]string{
			{"/tmp/default/1663/2", "/tmp/newprimary1/pg_tblspc/1663"},
		}
		originalRecreateSymLink := agent.ReCreateSymLink
		defer func() {
			agent.ReCreateSymLink = originalRecreateSymLink
		}()
		var actualRecreateSymLinkArgs [][]string
		agent.ReCreateSymLink = func(sourceDir, symLinkName string) error {
			actualRecreateSymLinkArgs = append(actualRecreateSymLinkArgs, []string{sourceDir, symLinkName})

			return nil
		}

		err := agent.RestoreTablespaces(request, segment)
		if err != nil {
			t.Errorf("got %+v, want nil", err)
		}

		if !reflect.DeepEqual(actualArgs, expectedRsyncArgs) {
			t.Errorf("rsync() invoked with %q, want %q", actualArgs, expectedRsyncArgs)
		}

		if !reflect.DeepEqual(actualRecreateSymLinkArgs, expectedRecreateSymLinkArgs) {
			t.Errorf("got %q, want %q", actualRecreateSymLinkArgs, expectedRecreateSymLinkArgs)
		}

		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()
	})

	t.Run("restore tablespace fails during rsync", func(t *testing.T) {
		request := &idl.UpgradePrimariesRequest{
			TablespacesMappingFilePath: "/tmp/tablespaces/tablespace_mapping.txt",
		}

		segment := getSampleSegment()

		agent.SetRsyncCommand(exectest.NewCommand(agent.FailedMain))
		defer func() { agent.SetRsyncCommand(nil) }()

		err := agent.RestoreTablespaces(request, segment)

		if err == nil {
			t.Error("expected Rsync() to fail")
		}

		expectedErrorStr := "rsync master tablespace directory"
		if !strings.Contains(err.Error(), expectedErrorStr) {
			t.Errorf("got %+v, want %+v", err, expectedErrorStr)
		}
	})

	t.Run("fails during recreation of symlink", func(t *testing.T) {
		request := &idl.UpgradePrimariesRequest{
			TablespacesMappingFilePath: "/tmp/tablespaces/tablespace_mapping.txt",
		}

		segment := getSampleSegment()

		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, errors.New("permission denied")
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		agent.SetRsyncCommand(exectest.NewCommand(agent.Success))
		defer func() { agent.SetRsyncCommand(nil) }()

		err := agent.RestoreTablespaces(request, segment)
		if err == nil {
			t.Error("expected ReCreateSymLink() to fail")
		}

		expectedErrorStr := "recreate symbolic link"
		if !strings.Contains(err.Error(), expectedErrorStr) {
			t.Errorf("got %+v, want %+v", err, expectedErrorStr)
		}
	})
}

func TestReCreateSymLink(t *testing.T) {
	t.Run("fails to stat symlink", func(t *testing.T) {
		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := agent.ReCreateSymLink("/tmp/masterdir", "masterLink")
		if err == nil {
			t.Errorf("got nil, want %+v", os.ErrPermission)
		}

		if !xerrors.Is(err, os.ErrPermission) {
			t.Errorf("expected error %#v to contain %#v", err, os.ErrPermission)
		}
	})

	t.Run("fails to remove symlink", func(t *testing.T) {
		utils.System.Remove = func(name string) error {
			return os.ErrPermission
		}

		statCalled := false
		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			statCalled = true
			return nil, nil
		}

		err := agent.ReCreateSymLink("/tmp/masterdir", "masterLink")
		if err == nil {
			t.Errorf("got nil, want %q", os.ErrPermission)
		}

		if !statCalled {
			t.Errorf("expected Lstat() to be called")
		}

		if os.ErrPermission != xerrors.Unwrap(err) {
			t.Errorf("got %q, want %q", err.Error(), os.ErrPermission)
		}

		expectedErrStr := "unlink"
		if !strings.Contains(err.Error(), expectedErrStr) {
			t.Errorf("got %q, want %q", err.Error(), expectedErrStr)
		}
	})

	t.Run("successfully creates symlink", func(t *testing.T) {
		symLinkCalled := false
		utils.System.Symlink = func(oldname, newname string) error {
			symLinkCalled = true
			return nil
		}

		statCalled := false
		utils.System.Lstat = func(name string) (os.FileInfo, error) {
			statCalled = true
			return nil, nil
		}

		removeCalled := false
		utils.System.Remove = func(name string) error {
			removeCalled = true
			return nil
		}

		err := agent.ReCreateSymLink("/tmp/masterdir", "masterLink")
		if err != nil {
			t.Errorf("got %+v, want nil", err)
		}

		if !statCalled {
			t.Errorf("expected Lstat() to be called")
		}

		if !removeCalled {
			t.Errorf("expected Remove() to be called")
		}

		if !symLinkCalled {
			t.Errorf("expected Symlink() to be called")
		}
	})
}
