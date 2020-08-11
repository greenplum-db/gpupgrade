// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/golang/mock/gomock"
	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/sys/unix"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func TestCheckDiskSpace(t *testing.T) {
	var d halfFullDisk
	var c *greenplum.Cluster
	var agents []*Connection
	var req *idl.CheckDiskSpaceRequest
	ctx := context.Background()

	testlog.SetupLogger()

	// This helper performs the boring test work. Set the above variables as
	// part of your more interesting test setup.
	check := func(t *testing.T, expected disk.SpaceFailures) {
		t.Helper()

		actual, err := checkDiskSpace(ctx, c, agents, d, req)
		if err != nil {
			t.Errorf("returned error %#v", err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("returned %v want %v", actual, expected)
		}
	}

	t.Run("reports no failures with enough space", func(t *testing.T) {
		c = MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/master", Role: "p"},
		})
		req = &idl.CheckDiskSpaceRequest{Ratio: 0.25}
		// leave agents empty

		check(t, disk.SpaceFailures{})
	})

	t.Run("reports disk failures for the master host", func(t *testing.T) {
		c = MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/master", Role: "p"},
		})
		req = &idl.CheckDiskSpaceRequest{Ratio: 0.75}
		// leave agents empty

		check(t, disk.SpaceFailures{
			"mdw: /": &idl.CheckDiskSpaceReply_DiskUsage{
				Required:  scale(d.Size(), 0.75),
				Available: scale(d.Size(), 0.5),
			},
		})
	})

	t.Run("reports disk failures from agent connections", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c = MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/master", Role: "p"},
			{ContentID: -1, Hostname: "smdw", DataDir: "/data/standby", Role: "m"},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/primary", Role: "p"},
			{ContentID: 1, Hostname: "sdw2", DataDir: "/data/primary", Role: "p"},
			{ContentID: 2, Hostname: "sdw2", DataDir: "/data/primary2", Role: "p"},
			{ContentID: 2, Hostname: "sdw3", DataDir: "/data/mirror2", Role: "m"},
		})
		req = &idl.CheckDiskSpaceRequest{Ratio: 0.25}

		// The usage descriptor returned by each mock agent. All we care is that
		// it's passed through untouched.
		usage := &idl.CheckDiskSpaceReply_DiskUsage{
			Required:  scale(d.Size(), 0.25),
			Available: scale(d.Size(), 0.5),
		}

		smdw := mock_idl.NewMockAgentClient(ctrl)
		smdw.EXPECT().
			CheckDiskSpace(ctx, &idl.CheckSegmentDiskSpaceRequest{
				Request:  req,
				Datadirs: []string{"/data/standby"},
			}).
			Return(&idl.CheckDiskSpaceReply{
				Failed: disk.SpaceFailures{"/": usage},
			}, nil)

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().
			CheckDiskSpace(ctx, &idl.CheckSegmentDiskSpaceRequest{
				Request:  req,
				Datadirs: []string{"/data/primary"},
			}).
			Return(&idl.CheckDiskSpaceReply{
				Failed: disk.SpaceFailures{"/": usage},
			}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().
			CheckDiskSpace(ctx, equivalentRequest(&idl.CheckSegmentDiskSpaceRequest{
				Request:  req,
				Datadirs: []string{"/data/primary", "/data/primary2"},
			})).
			Return(&idl.CheckDiskSpaceReply{
				Failed: disk.SpaceFailures{"/": usage},
			}, nil)

		sdw3 := mock_idl.NewMockAgentClient(ctrl)
		sdw3.EXPECT().
			CheckDiskSpace(ctx, equivalentRequest(&idl.CheckSegmentDiskSpaceRequest{
				Request:  req,
				Datadirs: []string{"/data/mirror2"},
			})).
			Return(&idl.CheckDiskSpaceReply{
				Failed: disk.SpaceFailures{"/": usage},
			}, nil)

		agents = []*Connection{
			{Hostname: "smdw", AgentClient: smdw},
			{Hostname: "sdw1", AgentClient: sdw1},
			{Hostname: "sdw2", AgentClient: sdw2},
			{Hostname: "sdw3", AgentClient: sdw3},
		}

		check(t, disk.SpaceFailures{
			"smdw: /": usage,
			"sdw1: /": usage,
			"sdw2: /": usage,
			"sdw3: /": usage,
		})
	})

	t.Run("bubbles up any errors in parallel", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c = MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", DataDir: "/data/master", Role: "p"},
			{ContentID: 0, Hostname: "sdw1", DataDir: "/data/primary", Role: "p"},
		})
		d.err = errors.New("master disk check is broken")
		// we don't care what req is for this case

		// One agent returns an error explicitly.
		agentErr := errors.New("agent connection is broken")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().
			CheckDiskSpace(ctx, &idl.CheckSegmentDiskSpaceRequest{
				Request:  req,
				Datadirs: []string{"/data/primary"},
			}).
			Return(nil, agentErr)

		// The other agent doesn't have any segments, so we expect no calls.
		sdw2 := mock_idl.NewMockAgentClient(ctrl)

		agents = []*Connection{
			{Hostname: "sdw1", AgentClient: sdw1},
			{Hostname: "sdw2", AgentClient: sdw2}, // invalid hostname
		}

		_, err := checkDiskSpace(ctx, c, agents, d, req)

		expected := []error{d.err, agentErr, greenplum.ErrUnknownHost}
		checkMultierrorContents(t, err, expected)
	})
}

// halfFullDisk is a stub implementation of disk.Disk. It has one 1MiB root
// filesystem with 50% utilization and no reserved space.
type halfFullDisk struct {
	err error
}

func (d halfFullDisk) Filesystems() (sigar.FileSystemList, error) {
	if d.err != nil {
		return sigar.FileSystemList{}, d.err
	}

	return sigar.FileSystemList{List: []sigar.FileSystem{
		{DirName: "/"},
	}}, nil
}

func (d halfFullDisk) Usage(path string) (sigar.FileSystemUsage, error) {
	if d.err != nil {
		return sigar.FileSystemUsage{}, d.err
	}

	u := sigar.FileSystemUsage{Total: d.Size()}

	u.Avail = scale(u.Total, 0.50)
	u.Used = u.Avail
	u.Free = u.Avail

	return u, nil
}

func (d halfFullDisk) Stat(path string) (*unix.Stat_t, error) {
	if d.err != nil {
		return nil, d.err
	}

	return &unix.Stat_t{Dev: 1}, nil
}

func (_ halfFullDisk) Size() uint64 {
	return 1024 * 1024
}

func scale(n uint64, f float64) uint64 {
	return uint64(float64(n) * f)
}

// checkMultierrorContents ensures that the passed error is actually a
// multierror.Error, and that it consists of exactly the expected contents
// (ignoring order).
func checkMultierrorContents(t *testing.T, err error, expected []error) {
	t.Helper()

	var multierr *multierror.Error
	if !errors.As(err, &multierr) {
		t.Errorf("error %#v does not contain type %T", err, multierr)
		return
	}

	// removes a single index from an error slice
	remove := func(i int, e []error) []error {
		return append(e[:i], e[i+1:]...)
	}

	failed := false
	for _, actual := range multierr.Errors {
		match := -1

		for i, candidate := range expected {
			if errors.Is(actual, candidate) {
				match = i
				break
			}
		}

		if match < 0 {
			t.Errorf("unexpected error %q", actual)
			failed = true
			continue
		}

		expected = remove(match, expected)
	}

	for _, missing := range expected {
		t.Errorf("did not find expected error %q", missing)
		failed = true
	}

	if failed {
		// Make the test easy to debug.
		t.Logf("actual error contents: %v", multierr)
	}
}

// equivalentRequest is a Matcher that can handle differences in order between
// two instances of CheckSegmentDiskSpaceRequest.Datadirs.
func equivalentRequest(req *idl.CheckSegmentDiskSpaceRequest) gomock.Matcher {
	return reqMatcher{req}
}

type reqMatcher struct {
	expected *idl.CheckSegmentDiskSpaceRequest
}

func (r reqMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.CheckSegmentDiskSpaceRequest)
	if !ok {
		return false
	}

	// The key here is that Datadirs can be in any order. Sort them before
	// comparison.
	sort.Strings(r.expected.Datadirs)
	sort.Strings(actual.Datadirs)

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}
