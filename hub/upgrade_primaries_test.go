// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestUpgradePrimaries(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},

		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},

		{DbID: 7, ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Port: 25437, Role: greenplum.PrimaryRole},
		{DbID: 8, ContentID: 2, Hostname: "sdw2", DataDir: "/data/dbfast_mirror3/seg3", Port: 25438, Role: greenplum.MirrorRole},
		{DbID: 9, ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast4/seg4", Port: 25439, Role: greenplum.PrimaryRole},
		{DbID: 10, ContentID: 3, Hostname: "sdw1", DataDir: "/data/dbfast_mirror4/seg4", Port: 25440, Role: greenplum.MirrorRole},
	})
	source.GPHome = "/usr/local/gpdb5"

	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 60432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 60433, Role: greenplum.MirrorRole},

		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 60434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 60435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 60436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 60437, Role: greenplum.MirrorRole},

		{DbID: 7, ContentID: 2, Hostname: "sdw1", DataDir: "/data/dbfast3/seg.HqtFHX54y0o.3", Port: 60438, Role: greenplum.PrimaryRole},
		{DbID: 8, ContentID: 2, Hostname: "sdw2", DataDir: "/data/dbfast_mirror3/seg.HqtFHX54y0o.3", Port: 60439, Role: greenplum.MirrorRole},
		{DbID: 9, ContentID: 3, Hostname: "sdw2", DataDir: "/data/dbfast4/seg.HqtFHX54y0o.4", Port: 60440, Role: greenplum.PrimaryRole},
		{DbID: 10, ContentID: 3, Hostname: "sdw1", DataDir: "/data/dbfast_mirror4/seg.HqtFHX54y0o.4", Port: 60441, Role: greenplum.MirrorRole},
	})
	intermediate.GPHome = "/usr/local/gpdb6"
	intermediate.Version = semver.MustParse("6.0.0")

	t.Run("calls upgrades primaries on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpgradePrimaries(
			gomock.Any(),
			equivalentUpgradePrimariesRequest(&idl.UpgradePrimariesRequest{
				Action: idl.PgOptions_check,
				Opts: []*idl.PgOptions{
					{
						Action:        idl.PgOptions_check,
						Role:          greenplum.PrimaryRole,
						ContentID:     0,
						Mode:          idl.PgOptions_Segment,
						LinkMode:      false,
						TargetVersion: "6.0.0",
						OldBinDir:     "/usr/local/gpdb5/bin",
						OldDataDir:    "/data/dbfast1/seg1",
						OldPort:       "25433",
						OldDBID:       "3",
						NewBinDir:     "/usr/local/gpdb6/bin",
						NewDataDir:    "/data/dbfast1/seg.HqtFHX54y0o.1",
						NewPort:       "60434",
						NewDBID:       "3",
						Tablespaces:   nil,
					},
					{
						Action:        idl.PgOptions_check,
						Role:          greenplum.PrimaryRole,
						ContentID:     2,
						Mode:          idl.PgOptions_Segment,
						LinkMode:      false,
						TargetVersion: "6.0.0",
						OldBinDir:     "/usr/local/gpdb5/bin",
						OldDataDir:    "/data/dbfast3/seg3",
						OldPort:       "25437",
						OldDBID:       "7",
						NewBinDir:     "/usr/local/gpdb6/bin",
						NewDataDir:    "/data/dbfast3/seg.HqtFHX54y0o.3",
						NewPort:       "60438",
						NewDBID:       "7",
						Tablespaces:   nil,
					},
				},
			}),
		).Return(&idl.UpgradePrimariesReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpgradePrimaries(
			gomock.Any(),
			equivalentUpgradePrimariesRequest(&idl.UpgradePrimariesRequest{
				Action: idl.PgOptions_check,
				Opts: []*idl.PgOptions{
					{
						Action:        idl.PgOptions_check,
						Role:          greenplum.PrimaryRole,
						ContentID:     1,
						Mode:          idl.PgOptions_Segment,
						LinkMode:      false,
						TargetVersion: "6.0.0",
						OldBinDir:     "/usr/local/gpdb5/bin",
						OldDataDir:    "/data/dbfast2/seg2",
						OldPort:       "25435",
						OldDBID:       "5",
						NewBinDir:     "/usr/local/gpdb6/bin",
						NewDataDir:    "/data/dbfast2/seg.HqtFHX54y0o.2",
						NewPort:       "60436",
						NewDBID:       "5",
						Tablespaces:   nil,
					},
					{
						Action:        idl.PgOptions_check,
						Role:          greenplum.PrimaryRole,
						ContentID:     3,
						Mode:          idl.PgOptions_Segment,
						LinkMode:      false,
						TargetVersion: "6.0.0",
						OldBinDir:     "/usr/local/gpdb5/bin",
						OldDataDir:    "/data/dbfast4/seg4",
						OldPort:       "25439",
						OldDBID:       "9",
						NewBinDir:     "/usr/local/gpdb6/bin",
						NewDataDir:    "/data/dbfast4/seg.HqtFHX54y0o.4",
						NewPort:       "60440",
						NewDBID:       "9",
						Tablespaces:   nil,
					},
				},
			}),
		).Return(&idl.UpgradePrimariesReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpgradePrimaries(agentConns, source, intermediate, idl.PgOptions_check, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	errCases := []struct {
		name   string
		Action idl.PgOptions_Action
		action string
	}{
		{
			name:   "returns error when failing to upgrade primaries on segments fails",
			Action: idl.PgOptions_upgrade,
			action: "upgrade",
		},
		{
			name:   "returns error when failing to check primaries on segments fails",
			Action: idl.PgOptions_check,
			action: "check",
		},
	}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			expected := os.ErrPermission
			sdw1 := mock_idl.NewMockAgentClient(ctrl)
			sdw1.EXPECT().UpgradePrimaries(
				gomock.Any(),
				gomock.Any(),
			).Return(nil, expected)

			sdw2 := mock_idl.NewMockAgentClient(ctrl)
			sdw2.EXPECT().UpgradePrimaries(
				gomock.Any(),
				gomock.Any(),
			).Return(nil, expected)

			agentConns := []*idl.Connection{
				{AgentClient: sdw1, Hostname: "sdw1"},
				{AgentClient: sdw2, Hostname: "sdw2"},
			}

			err := hub.UpgradePrimaries(agentConns, source, intermediate, c.Action, true)
			var errs errorlist.Errors
			if !xerrors.As(err, &errs) {
				t.Fatalf("error %#v does not contain type %T", err, errs)
			}

			if len(errs) != 2 {
				t.Fatalf("got error count %d, want %d", len(errs), 2)
			}

			sort.Sort(errs)
			for i, err := range errs {
				if !errors.Is(err, expected) {
					t.Errorf("got error %#v, want %#v", err, expected)
				}

				// XXX it'd be nice if we didn't couple against a hardcoded string here,
				// but it's difficult to unwrap multiple errors with the new xerrors interface.
				expectedErrMsg := fmt.Errorf("%s primary segment on host sdw%d: %w", c.action, i+1, expected)
				if err.Error() != expectedErrMsg.Error() {
					t.Errorf("got %q want %q", err.Error(), expectedErrMsg)
				}
			}
		})
	}
}

// equivalentUpgradePrimariesRequest is a Matcher that can handle differences in order between
// two instances of DeleteTablespaceRequest.Dirs
func equivalentUpgradePrimariesRequest(req *idl.UpgradePrimariesRequest) gomock.Matcher {
	return reqUpgradePrimariesMatcher{req}
}

type reqUpgradePrimariesMatcher struct {
	expected *idl.UpgradePrimariesRequest
}

func (r reqUpgradePrimariesMatcher) Matches(x interface{}) bool {
	actual, ok := x.(*idl.UpgradePrimariesRequest)
	if !ok {
		return false
	}

	// The key here is that getOpts can be in any order. Sort them before comparison.
	sort.Sort(getOpts(r.expected.GetOpts()))
	sort.Sort(getOpts(actual.GetOpts()))

	return reflect.DeepEqual(r.expected, actual)
}

func (r reqUpgradePrimariesMatcher) String() string {
	return fmt.Sprintf("is equivalent to %v", r.expected)
}

type getOpts []*idl.PgOptions

func (r getOpts) Len() int {
	return len(r)
}

func (r getOpts) Less(i, j int) bool {
	return r[i].GetContentID() > r[j].GetContentID()
}

func (r getOpts) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
