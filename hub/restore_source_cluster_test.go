// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

func ResetRecoversegCmd() {
	hub.RecoversegCmd = exec.Command
}

func TestRsyncMasterAndPrimaries(t *testing.T) {
	testlog.SetupLogger()

	cluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "msdw2", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})
	cluster.GPHome = "/usr/local/greenplum-db"
	cluster.Version = semver.MustParse("5.0.0")

	tablespaces := testutils.CreateTablespaces()

	t.Run("restores master in link mode using correct rsync arguments", func(t *testing.T) {
		defer rsync.ResetRsyncCommand()
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:3]
			if !reflect.DeepEqual(options, hub.Options) {
				t.Errorf("got options %q want %q", options, hub.Options)
			}

			source := args[3]
			expected := "standby:/data/standby/"
			if source != expected {
				t.Errorf("got source %q want %q", source, expected)
			}

			destination := args[4]
			expected = "/data/qddir"
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

	t.Run("errors in restoring tablespaces when source cluster does not have mirrors and standby", func(t *testing.T) {
		cluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		err := hub.RsyncMasterAndPrimariesTablespaces(&testutils.DevNullWithClose{}, []*idl.Connection{}, cluster)
		if !errors.Is(err, hub.ErrMissingMirrorsAndStandby) {
			t.Errorf("got error %#v want %#v", err, hub.ErrMissingMirrorsAndStandby)
		}
	})

	t.Run("restores master tablespaces in link mode using correct rsync arguments", func(t *testing.T) {
		defer rsync.ResetRsyncCommand()
		rsync.SetRsyncCommand(exectest.NewCommandWithVerifier(hub.Success, func(utility string, args ...string) {
			if !strings.HasSuffix(utility, "rsync") {
				t.Errorf("got %q want rsync", utility)
			}

			options := args[:3]
			if !reflect.DeepEqual(options, hub.Options) {
				t.Errorf("got options %q want %q", options, hub.Options)
			}

			source := args[3]
			expected := "standby:/tmp/user_ts/m/standby/16384/"
			if source != expected {
				t.Errorf("got source %q want %q", source, expected)
			}

			destination := args[4]
			expected = "/tmp/user_ts/m/qddir/16384"
			if destination != expected {
				t.Errorf("got destination %q want %q", destination, expected)
			}
		}))

		err := hub.RsyncMasterTablespaces(&testutils.DevNullWithClose{}, cluster.StandbyHostname(), tablespaces[cluster.Master().DbID], tablespaces[cluster.Standby().DbID])
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

			expected := []string{"-c", fmt.Sprintf("source /usr/local/greenplum-db/greenplum_path.sh && MASTER_DATA_DIRECTORY=%s PGPORT=%d "+
				"/usr/local/greenplum-db/bin/gprecoverseg -a --hba-hostnames", cluster.MasterDataDir(), cluster.MasterPort())}
			if !reflect.DeepEqual(args, expected) {
				t.Errorf("got %q want %q", args, expected)
			}
		})

		err := hub.Recoverseg(&testutils.DevNullWithClose{}, cluster, true)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("restores primaries using correct gRPC arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{{
					Sources:         []string{"/data/dbfast_mirror1/seg1" + string(os.PathSeparator)},
					DestinationHost: "sdw1",
					Destination:     "/data/dbfast1/seg1",
					Options:         hub.Options,
					ExcludedFiles:   hub.Excludes,
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		msdw2 := mock_idl.NewMockAgentClient(ctrl)
		msdw2.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{{
					Sources:         []string{"/data/dbfast_mirror2/seg2" + string(os.PathSeparator)},
					DestinationHost: "sdw2",
					Destination:     "/data/dbfast2/seg2",
					Options:         hub.Options,
					ExcludedFiles:   hub.Excludes,
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		standby := mock_idl.NewMockAgentClient(ctrl)

		agentConns := []*idl.Connection{
			{AgentClient: msdw1, Hostname: "msdw1"},
			{AgentClient: msdw2, Hostname: "msdw2"},
			{AgentClient: standby, Hostname: "standby"},
		}

		err := hub.RsyncPrimaries(agentConns, cluster)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("restores primary tablespaces using correct gRPC arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{{
					Sources:         []string{"/tmp/user_ts/m1/16384" + string(os.PathSeparator)},
					DestinationHost: "sdw1",
					Destination:     "/tmp/user_ts/p1/16384",
					Options:         hub.Options,
					ExcludedFiles:   hub.Excludes,
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		msdw2 := mock_idl.NewMockAgentClient(ctrl)
		msdw2.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			&idl.RsyncRequest{
				Options: []*idl.RsyncRequest_RsyncOptions{{
					Sources:         []string{"/tmp/user_ts/m2/16384" + string(os.PathSeparator)},
					DestinationHost: "sdw2",
					Destination:     "/tmp/user_ts/p2/16384",
					Options:         hub.Options,
					ExcludedFiles:   hub.Excludes,
				}},
			},
		).Return(&idl.RsyncReply{}, nil)

		standby := mock_idl.NewMockAgentClient(ctrl)

		agentConns := []*idl.Connection{
			{AgentClient: msdw1, Hostname: "msdw1"},
			{AgentClient: msdw2, Hostname: "msdw2"},
			{AgentClient: standby, Hostname: "standby"},
		}

		err := hub.RsyncPrimariesTablespaces(agentConns, cluster, tablespaces)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("errors when source cluster does not have all mirrors and standby", func(t *testing.T) {
		cluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		})

		err := hub.RsyncMasterAndPrimaries(&testutils.DevNullWithClose{}, []*idl.Connection{}, cluster)
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

	t.Run("errors when restoring the master tablespaces fails in link mode", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(hub.Failure))
		defer rsync.ResetRsyncCommand()

		err := hub.RsyncMasterTablespaces(&testutils.DevNullWithClose{}, cluster.MasterHostname(), tablespaces[greenplum.MasterDbid], tablespaces[cluster.Standby().DbID])
		if err == nil {
			t.Error("unexpected nil error")
		}
	})

	t.Run("errors when restoring the mirrors fails in copy mode on GPDB5", func(t *testing.T) {
		defer ResetRecoversegCmd()
		hub.RecoversegCmd = exectest.NewCommand(hub.Failure)

		err := hub.Recoverseg(&testutils.DevNullWithClose{}, cluster, false)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Errorf("returned error %#v, want exit code %d", err, 1)
		}
	})

	t.Run("errors when restoring the primaries fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.RsyncReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RsyncDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: msdw1, Hostname: "msdw1"},
			{AgentClient: failedClient, Hostname: "msdw2"},
		}

		err := hub.RsyncPrimaries(agentConns, cluster)

		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("errors when restoring the primaries tablespaces fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		msdw1 := mock_idl.NewMockAgentClient(ctrl)
		msdw1.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.RsyncReply{}, nil)

		expected := errors.New("permission denied")
		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RsyncTablespaceDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: msdw1, Hostname: "msdw1"},
			{AgentClient: failedClient, Hostname: "msdw2"},
		}

		err := hub.RsyncPrimariesTablespaces(agentConns, cluster, tablespaces)

		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

// RestoreMasterAndPrimariesPgControl invokes the restoration of pg_control on
// master and segments. So, not testing pg_control restoration on segments separately.
func TestRestoreMasterAndPrimariesPgControl(t *testing.T) {
	testlog.SetupLogger()

	t.Run("errors when restoring pg_control on the master and primaries fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "master", DataDir: "/data/qddir", Role: greenplum.PrimaryRole},
			{ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
			{ContentID: 1, Hostname: "msdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
			{ContentID: 2, Hostname: "sdw2", DataDir: "/data/dbfast3/seg3", Role: greenplum.PrimaryRole},
			{ContentID: 2, Hostname: "msdw2", DataDir: "/data/dbfast_mirror3/seg3", Role: greenplum.MirrorRole},
			{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast4/seg4", Role: greenplum.PrimaryRole},
			{ContentID: 3, Hostname: "msdw2", DataDir: "/data/dbfast_mirror4/seg4", Role: greenplum.MirrorRole},
		})

		expectedError := os.ErrNotExist

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RestorePrimariesPgControl(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.RestorePgControlReply{}, nil)

		failedClient := mock_idl.NewMockAgentClient(ctrl)
		failedClient.EXPECT().RestorePrimariesPgControl(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expectedError)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: failedClient, Hostname: "sdw2"},
		}

		err := hub.RestoreMasterAndPrimariesPgControl(step.DevNullStream, agentConns, cluster)

		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Errorf("received %d errors, want %d", len(errs), 1)
		}

		for _, err := range errs {
			if !errors.Is(err, expectedError) {
				t.Errorf("got error %#v, want %#v", expectedError, err)
			}
		}
	})

	t.Run("restores master and primaries pg_control successfully using correct arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		masterDir := testutils.GetTempDir(t, "")
		cluster := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "master", DataDir: masterDir, Role: greenplum.PrimaryRole},
			{ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Role: greenplum.MirrorRole},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "msdw1", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
			{ContentID: 1, Hostname: "msdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
			{ContentID: 2, Hostname: "sdw2", DataDir: "/data/dbfast3/seg3", Role: greenplum.PrimaryRole},
			{ContentID: 2, Hostname: "msdw2", DataDir: "/data/dbfast_mirror3/seg3", Role: greenplum.MirrorRole},
			{ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast4/seg4", Role: greenplum.PrimaryRole},
			{ContentID: 3, Hostname: "msdw2", DataDir: "/data/dbfast_mirror4/seg4", Role: greenplum.MirrorRole},
		})

		globalDir := filepath.Join(masterDir, "global")
		err := os.Mkdir(globalDir, 0700)
		if err != nil {
			t.Fatalf("failed to create directory %s: %#v", globalDir, err)
		}

		file := filepath.Join(globalDir, "pg_control.old")
		_, err = os.Create(file)
		if err != nil {
			t.Fatalf("failed to create file %s: %#v", file, err)
		}

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().RestorePrimariesPgControl(
			gomock.Any(),
			equivalentRestorePgControlRequest(&idl.RestorePgControlRequest{
				Datadirs: []string{"/data/dbfast1/seg1", "/data/dbfast2/seg2"},
			},
			)).Return(&idl.RestorePgControlReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().RestorePrimariesPgControl(
			gomock.Any(),
			equivalentRestorePgControlRequest(&idl.RestorePgControlRequest{
				Datadirs: []string{"/data/dbfast3/seg3", "/data/dbfast4/seg4"},
			},
			)).Return(&idl.RestorePgControlReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err = hub.RestoreMasterAndPrimariesPgControl(step.DevNullStream, agentConns, cluster)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})
}

// equivalentRestorePgControlRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentRestorePgControlRequest(req *idl.RestorePgControlRequest) gomock.Matcher {
	return reqRestorePgControlMatcher{req}
}

type reqRestorePgControlMatcher struct {
	expected *idl.RestorePgControlRequest
}

func (r reqRestorePgControlMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.RestorePgControlRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Strings(r.expected.GetDatadirs())
	sort.Strings(actual.GetDatadirs())

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqRestorePgControlMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}
