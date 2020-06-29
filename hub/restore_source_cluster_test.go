// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func ResetRecoversegCmd() {
	hub.RecoversegCmd = exec.Command
}

func TestRsyncMasterAndPrimaries(t *testing.T) {
	testhelper.SetupTestLogger()

	cluster := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
		{ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
		{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 1, Hostname: "msdw2", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})
	cluster.BinDir = "/usr/local/greenplum-db/bin"
	cluster.Version = dbconn.NewVersion("5.0.0")

	t.Run("restores master in link mode using correct rsync arguments", func(t *testing.T) {
		defer rsync.ResetRsyncCommand()
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			if utility != "rsync" {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:3]
			if !reflect.DeepEqual(options, hub.Options) {
				t.Errorf("got options %q want %q", options, hub.Options)
			}

			source := args[3]
			expected := "/data/standby/"
			if source != expected {
				t.Errorf("got source %q want %q", source, expected)
			}

			destination := args[4]
			expected = "master:/data/qddir"
			if destination != expected {
				t.Errorf("got destination %q want %q", destination, expected)
			}

			excludes := strings.Join(args[6:], " ")
			expected = strings.Join(hub.Excludes, " --exclude ")
			if !reflect.DeepEqual(excludes, expected) {
				t.Errorf("got exclusions %q want %q", excludes, expected)
			}
		}))

		err := hub.RsyncMaster(&testutils.DevNullWithClose{}, cluster.Standby(), cluster.Master())
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("restores mirrors in copy mode on GPDB5", func(t *testing.T) {
		defer ResetRecoversegCmd()
		hub.RecoversegCmd = exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			if utility != "bash" {
				t.Errorf("got %q want bash", utility)
			}

			expected := []string{"-c", "source /usr/local/greenplum-db/greenplum_path.sh && /usr/local/greenplum-db/bin/gprecoverseg -a"}
			if !reflect.DeepEqual(args, expected) {
				t.Errorf("got %q want %q", args, expected)
			}
		})

		err := hub.Recoverseg(&testutils.DevNullWithClose{}, cluster)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("does not restore the mirrors in copy mode on GPDB6 or higher", func(t *testing.T) {
		defer func() {
			cluster.Version = dbconn.NewVersion("5.0.0")
		}()

		cluster.Version = dbconn.NewVersion("6.0.0")
		called := false

		defer ResetRecoversegCmd()
		hub.RecoversegCmd = exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			called = true
		})

		err := hub.Recoverseg(&testutils.DevNullWithClose{}, cluster)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if called {
			t.Errorf("expected gprecoverseg to not be called")
		}
	})

	t.Run("restores primaries using correct gRPC arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().Rsync(
			gomock.Any(),
			&idl.RsyncRequest{
				Options:  hub.Options,
				Excludes: hub.Excludes,
				Pairs: []*idl.RsyncPair{{
					Source:      "/data/dbfast_mirror1/seg1" + string(os.PathSeparator),
					RemoteHost:  "sdw1",
					Destination: "/data/dbfast1/seg1",
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		msdw2 := mock_idl.NewMockAgentClient(ctrl)
		msdw2.EXPECT().Rsync(
			gomock.Any(),
			&idl.RsyncRequest{
				Options:  hub.Options,
				Excludes: hub.Excludes,
				Pairs: []*idl.RsyncPair{{
					Source:      "/data/dbfast_mirror2/seg2" + string(os.PathSeparator),
					RemoteHost:  "sdw2",
					Destination: "/data/dbfast2/seg2",
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		standby := mock_idl.NewMockAgentClient(ctrl)

		agentConns := []*hub.Connection{
			{nil, msdw1, "msdw1", nil},
			{nil, msdw2, "msdw2", nil},
			{nil, standby, "standby", nil},
		}

		err := hub.RsyncPrimaries(agentConns, cluster)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when source cluster does not have all mirrors and standby", func(t *testing.T) {
		cluster := hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		err := hub.RsyncMasterAndPrimaries(&testutils.DevNullWithClose{}, []*hub.Connection{}, cluster)
		if err == nil {
			t.Error("unexpected nil error")
		}
	})

	t.Run("errors when restoring the master fails in link mode", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(hub.Failure))
		defer rsync.ResetRsyncCommand()

		err := hub.RsyncMaster(&testutils.DevNullWithClose{}, cluster.Standby(), cluster.Master())
		if err == nil {
			t.Error("unexpected nil error")
		}
	})

	t.Run("errors when restoring the mirrors fails in copy mode on GPDB5", func(t *testing.T) {
		defer ResetRecoversegCmd()
		hub.RecoversegCmd = exectest.NewCommand(hub.Failure)

		err := hub.Recoverseg(&testutils.DevNullWithClose{}, cluster)
		var exitErr *exec.ExitError
		if !xerrors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Errorf("returned error %#v, want exit code %d", err, 1)
		}
	})

	t.Run("errors when restoring the primaries fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().Rsync(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.RsyncReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().Rsync(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*hub.Connection{
			{nil, msdw1, "msdw1", nil},
			{nil, failedClient, "msdw2", nil},
		}

		err := hub.RsyncPrimaries(agentConns, cluster)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})
}
