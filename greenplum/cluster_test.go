// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestCluster(t *testing.T) {
	primaries := map[int]greenplum.SegConfig{
		-1: {DbID: 1, ContentID: -1, Port: 5432, Hostname: "localhost", DataDir: "/data/gpseg-1"},
		0:  {DbID: 2, ContentID: 0, Port: 20000, Hostname: "localhost", DataDir: "/data/gpseg0"},
		2:  {DbID: 4, ContentID: 2, Port: 20002, Hostname: "localhost", DataDir: "/data/gpseg2"},
		3:  {DbID: 5, ContentID: 3, Port: 20003, Hostname: "remotehost2", DataDir: "/data/gpseg3"},
	}
	for content, seg := range primaries {
		seg.Role = greenplum.PrimaryRole
		primaries[content] = seg
	}

	mirrors := map[int]greenplum.SegConfig{
		-1: {DbID: 8, ContentID: -1, Port: 5433, Hostname: "localhost", DataDir: "/mirror/gpseg-1"},
		0:  {DbID: 3, ContentID: 0, Port: 20001, Hostname: "localhost", DataDir: "/mirror/gpseg0"},
		2:  {DbID: 6, ContentID: 2, Port: 20004, Hostname: "localhost", DataDir: "/mirror/gpseg2"},
		3:  {DbID: 7, ContentID: 3, Port: 20005, Hostname: "remotehost2", DataDir: "/mirror/gpseg3"},
	}
	for content, seg := range mirrors {
		seg.Role = greenplum.MirrorRole
		mirrors[content] = seg
	}

	master := primaries[-1]
	standby := mirrors[-1]

	cases := []struct {
		name      string
		primaries []greenplum.SegConfig
		mirrors   []greenplum.SegConfig
	}{
		{"mirrorless single-host, single-segment", []greenplum.SegConfig{master, primaries[0]}, nil},
		{"mirrorless single-host, multi-segment", []greenplum.SegConfig{master, primaries[0], primaries[2]}, nil},
		{"mirrorless multi-host, multi-segment", []greenplum.SegConfig{master, primaries[0], primaries[3]}, nil},
		{"single-host, single-segment",
			[]greenplum.SegConfig{master, primaries[0]},
			[]greenplum.SegConfig{mirrors[0]},
		},
		{"single-host, multi-segment",
			[]greenplum.SegConfig{master, primaries[0], primaries[2]},
			[]greenplum.SegConfig{mirrors[0], mirrors[2]},
		},
		{"multi-host, multi-segment",
			[]greenplum.SegConfig{master, primaries[0], primaries[3]},
			[]greenplum.SegConfig{mirrors[0], mirrors[3]},
		},
		{"multi-host, multi-segment with standby",
			[]greenplum.SegConfig{master, primaries[0], primaries[3]},
			[]greenplum.SegConfig{standby, mirrors[0], mirrors[3]},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s cluster", c.name), func(t *testing.T) {
			segments := append(c.primaries, c.mirrors...)

			actualCluster := greenplum.MustCreateCluster(t, segments)
			actualContents := actualCluster.GetContentList()

			var expectedContents []int
			for _, p := range c.primaries {
				expectedContents = append(expectedContents, p.ContentID)
			}

			if !reflect.DeepEqual(actualContents, expectedContents) {
				t.Errorf("GetContentList() = %v, want %v", actualContents, expectedContents)
			}

			for _, expected := range c.primaries {
				content := expected.ContentID

				actual := actualCluster.Primaries[content]
				if actual != expected {
					t.Errorf("Primaries[%d] = %+v, want %+v", content, actual, expected)
				}

				host := actualCluster.GetHostForContent(content)
				if host != expected.Hostname {
					t.Errorf("GetHostForContent(%d) = %q, want %q", content, host, expected.Hostname)
				}

				port := actualCluster.GetPortForContent(content)
				if port != expected.Port {
					t.Errorf("GetPortForContent(%d) = %d, want %d", content, port, expected.Port)
				}

				dbid := actualCluster.GetDbidForContent(content)
				if dbid != expected.DbID {
					t.Errorf("GetDbidForContent(%d) = %d, want %d", content, dbid, expected.DbID)
				}

				datadir := actualCluster.GetDirForContent(content)
				if datadir != expected.DataDir {
					t.Errorf("GetDirForContent(%d) = %q, want %q", content, datadir, expected.DataDir)
				}
			}

			for _, expected := range c.mirrors {
				content := expected.ContentID

				actual := actualCluster.Mirrors[content]
				if actual != expected {
					t.Errorf("Mirrors[%d] = %+v, want %+v", content, actual, expected)
				}
			}
		})
	}

	errCases := []struct {
		name     string
		segments []greenplum.SegConfig
	}{
		{"bad role", []greenplum.SegConfig{
			{Role: "x"},
		}},
		{"mirror without primary", []greenplum.SegConfig{
			{ContentID: 0, Role: "p"},
			{ContentID: 1, Role: "m"},
		}},
		{"duplicated primary contents", []greenplum.SegConfig{
			{ContentID: 0, Role: "p"},
			{ContentID: 0, Role: "p"},
		}},
		{"duplicated mirror contents", []greenplum.SegConfig{
			{ContentID: 0, Role: "p"},
			{ContentID: 0, Role: "m"},
			{ContentID: 0, Role: "m"},
		}},
	}

	for _, c := range errCases {
		t.Run(fmt.Sprintf("doesn't allow %s", c.name), func(t *testing.T) {
			_, err := greenplum.NewCluster(c.segments)

			if !errors.Is(err, greenplum.ErrInvalidSegments) {
				t.Errorf("returned error %#v, want %#v", err, greenplum.ErrInvalidSegments)
			}
		})
	}
}

func TestGetSegmentConfiguration(t *testing.T) {
	t.Run("can retrieve gp_segment_configuration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "datadir", "role"})
		rows.AddRow(1, -1, 15432, "mdw", "/data/qddir/seg-1", greenplum.PrimaryRole)
		rows.AddRow(2, -1, 16432, "smdw", "/data/standby", greenplum.MirrorRole)
		rows.AddRow(3, 0, 25433, "sdw1", "/data/dbfast1/seg1", greenplum.PrimaryRole)
		rows.AddRow(4, 0, 25434, "sdw2", "/data/dbfast_mirror1/seg1", greenplum.MirrorRole)
		rows.AddRow(5, 1, 25435, "sdw2", "/data/dbfast2/seg2", greenplum.PrimaryRole)
		rows.AddRow(6, 1, 25436, "sdw1", "/data/dbfast_mirror2/seg2", greenplum.MirrorRole)

		mock.ExpectQuery("SELECT").WillReturnRows(rows)

		actual, err := greenplum.GetSegmentConfiguration(db, semver.Version{})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		expected := []greenplum.SegConfig{
			{DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Port: 16432, Hostname: "smdw", DataDir: "/data/standby", Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Port: 25433, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Port: 25434, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Port: 25435, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Port: 25436, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
		}

		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("got configuration %+v, want %+v", actual, expected)
		}
	})

	t.Run("can retrieve gp_segment_configuration when all segements are on same host", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "datadir", "role"})
		rows.AddRow(1, -1, 15432, "mdw", "/data/qddir/seg-1", greenplum.PrimaryRole)
		rows.AddRow(2, -1, 16432, "mdw", "/data/standby", greenplum.MirrorRole)
		rows.AddRow(3, 0, 25433, "mdw", "/data/dbfast1/seg1", greenplum.PrimaryRole)
		rows.AddRow(4, 0, 25434, "mdw", "/data/dbfast_mirror1/seg1", greenplum.MirrorRole)
		rows.AddRow(5, 1, 25435, "mdw", "/data/dbfast2/seg2", greenplum.PrimaryRole)
		rows.AddRow(6, 1, 25436, "mdw", "/data/dbfast_mirror2/seg2", greenplum.MirrorRole)

		mock.ExpectQuery("SELECT").WillReturnRows(rows)

		actual, err := greenplum.GetSegmentConfiguration(db, semver.Version{})
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		expected := []greenplum.SegConfig{
			{DbID: 1, ContentID: -1, Port: 15432, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Port: 16432, Hostname: "mdw", DataDir: "/data/standby", Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Port: 25433, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Port: 25434, Hostname: "mdw", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Port: 25435, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Port: 25436, Hostname: "mdw", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
		}

		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("got configuration %+v, want %+v", actual, expected)
		}
	})
}

func TestPrimaryHostnames(t *testing.T) {
	testStateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("got error when creating tempdir: %+v", err)
	}
	expectedCluster := testutils.CreateMultinodeSampleCluster("/tmp")
	expectedCluster.GPHome = "/fake/path"
	expectedCluster.Version = semver.MustParse("6.0.0")
	testlog.SetupLogger()

	defer func() {
		os.RemoveAll(testStateDir)
	}()

	t.Run("returns a list of hosts for only the primaries", func(t *testing.T) {
		actual := expectedCluster.PrimaryHostnames()
		sort.Strings(actual)

		expected := []string{"host1", "host2"}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("expected hostnames: %#v got: %#v", expected, actual)
		}
	})
}

func TestClusterFromDB(t *testing.T) {
	testStateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("got error when creating tempdir: %+v", err)
	}

	testlog.SetupLogger()

	defer func() {
		os.RemoveAll(testStateDir)
	}()

	t.Run("returns an error if connection fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		expected := errors.New("connection failed")
		mock.ExpectQuery("SELECT ").WillReturnError(expected)

		actualCluster, err := greenplum.ClusterFromDB(db, semver.MustParse("0.0.0"), "", idl.ClusterDestination_SOURCE)
		if !errors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}

		if !reflect.DeepEqual(actualCluster, greenplum.Cluster{}) {
			t.Errorf("got: %#v want empty cluster: %#v", actualCluster, &greenplum.Cluster{})
		}
	})

	t.Run("returns an error if the segment configuration query fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		queryErr := errors.New("failed to get segment configuration")
		mock.ExpectQuery("SELECT .* FROM gp_segment_configuration").WillReturnError(queryErr)

		actualCluster, err := greenplum.ClusterFromDB(db, semver.MustParse("0.0.0"), "", idl.ClusterDestination_SOURCE)

		if err == nil {
			t.Errorf("Expected an error, but got nil")
		}
		if !reflect.DeepEqual(actualCluster, greenplum.Cluster{}) {
			t.Errorf("Expected cluster to be empty, but got %#v", actualCluster)
		}
		if !strings.Contains(err.Error(), queryErr.Error()) {
			t.Errorf("Expected error: %+v got: %+v", queryErr.Error(), err.Error())
		}
	})

	t.Run("populates a cluster using DB information", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("couldn't create sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)

		mock.ExpectQuery("SELECT .* FROM gp_segment_configuration").WillReturnRows(testutils.MockSegmentConfiguration())

		gphome := "/usr/local/gpdb"
		version := semver.MustParse("5.3.4")
		destination := idl.ClusterDestination_INTERMEDIATE
		actualCluster, err := greenplum.ClusterFromDB(db, version, gphome, destination)
		if err != nil {
			t.Errorf("got unexpected error: %+v", err)
		}

		expectedCluster := testutils.MockCluster()
		expectedCluster.Destination = destination
		expectedCluster.Version = version
		expectedCluster.GPHome = gphome

		if !reflect.DeepEqual(&actualCluster, expectedCluster) {
			t.Errorf("got: %#v want: %#v ", &actualCluster, expectedCluster)
		}
	})
}

func TestSelectSegments(t *testing.T) {
	segs := []greenplum.SegConfig{
		{ContentID: 1, Role: "p"},
		{ContentID: 2, Role: "p"},
		{ContentID: 3, Role: "p"},
		{ContentID: 3, Role: "m"},
	}
	cluster := greenplum.MustCreateCluster(t, segs)

	// Ensure all segments are visited correctly.
	selectAll := func(_ *greenplum.SegConfig) bool { return true }
	results := cluster.SelectSegments(selectAll)

	if !reflect.DeepEqual(results, segs) {
		t.Errorf("SelectSegments(*) = %+v, want %+v", results, segs)
	}

	// Test a simple selector.
	moreThanOne := func(c *greenplum.SegConfig) bool { return c.ContentID > 1 }
	results = cluster.SelectSegments(moreThanOne)

	expected := []greenplum.SegConfig{segs[1], segs[2], segs[3]}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("SelectSegments(ContentID > 1) = %+v, want %+v", results, expected)
	}

}

func TestHasAllMirrorsAndStandby(t *testing.T) {
	t.Run("returns true on full cluster", func(t *testing.T) {
		segs := []greenplum.SegConfig{
			{ContentID: -1, Role: "p"},
			{ContentID: -1, Role: "m"},
			{ContentID: 0, Role: "p"},
			{ContentID: 0, Role: "m"},
			{ContentID: 1, Role: "p"},
			{ContentID: 1, Role: "m"},
			{ContentID: 2, Role: "p"},
			{ContentID: 2, Role: "m"},
		}
		cluster := greenplum.MustCreateCluster(t, segs)

		if !cluster.HasAllMirrorsAndStandby() {
			t.Errorf("expected a cluster that has all mirrors and a standby")
		}
	})

	cases := []struct {
		name string
		segs []greenplum.SegConfig
	}{
		{
			"returns false on cluster with no mirrors",
			[]greenplum.SegConfig{
				{ContentID: -1, Role: "p"},
				{ContentID: 0, Role: "p"},
				{ContentID: 1, Role: "p"},
				{ContentID: 2, Role: "p"},
			},
		},
		{
			"returns false on cluster with mirrors but no standby",
			[]greenplum.SegConfig{
				{ContentID: -1, Role: "p"},
				{ContentID: 0, Role: "p"},
				{ContentID: 0, Role: "m"},
				{ContentID: 1, Role: "p"},
				{ContentID: 1, Role: "m"},
				{ContentID: 2, Role: "p"},
				{ContentID: 2, Role: "m"},
			},
		},
		{
			"returns false on cluster with standby and no mirrors",
			[]greenplum.SegConfig{
				{ContentID: -1, Role: "p"},
				{ContentID: -1, Role: "m"},
				{ContentID: 0, Role: "p"},
				{ContentID: 1, Role: "p"},
				{ContentID: 2, Role: "p"},
			},
		},
		{
			"returns false on cluster with only one mirror",
			[]greenplum.SegConfig{
				{ContentID: -1, Role: "p"},
				{ContentID: 0, Role: "p"},
				{ContentID: 0, Role: "m"},
				{ContentID: 1, Role: "p"},
				{ContentID: 2, Role: "p"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cluster := greenplum.MustCreateCluster(t, c.segs)

			if cluster.HasAllMirrorsAndStandby() {
				t.Errorf("expected a cluster missing at least one mirror or its standby")
			}
		})
	}
}

func TestWaitForSegments(t *testing.T) {
	timeout := 30 * time.Second

	target := MustCreateCluster(t, []greenplum.SegConfig{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("succeeds", func(t *testing.T) {
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 4)
		expectGpStatReplicationToReturn(mock, 1)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("skips fts and gp_stat_replication if GPDB version is 5", func(t *testing.T) {
		target.Version = semver.MustParse("5.0.0")

		expectGpSegmentConfigurationToReturn(mock, 4)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("skips gp_stat_replication if there is no standby", func(t *testing.T) {
		target := MustCreateCluster(t, []greenplum.SegConfig{
			{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
		})
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 4)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("does not check mode=s if there are no mirrors but has a standby", func(t *testing.T) {
		target := MustCreateCluster(t, []greenplum.SegConfig{
			{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		})
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationWithoutMirrorsToReturn(mock, 2)
		expectGpStatReplicationToReturn(mock, 1)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("skips mode=s and gp_stat_replication checks if there are no mirrors and no standby", func(t *testing.T) {
		target := MustCreateCluster(t, []greenplum.SegConfig{
			{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		})
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationWithoutMirrorsToReturn(mock, 2)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("waits for segments to come up and standby to be synchronized", func(t *testing.T) {
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 0)
		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 4)
		expectGpStatReplicationToReturn(mock, 0)
		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 4)
		expectGpStatReplicationToReturn(mock, 1)

		err = greenplum.WaitForSegments(db, timeout, target)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("times out if segments never come up", func(t *testing.T) {
		target.Version = semver.MustParse("6.0.0")

		expectFtsProbe(mock)
		expectGpSegmentConfigurationToReturn(mock, 0)

		err = greenplum.WaitForSegments(db, -1*time.Second, target)
		expected := "-1s timeout exceeded waiting for all segments to be up, in their preferred roles, and synchronized."
		if err.Error() != expected {
			t.Errorf("got: %#v want %s", err, expected)
		}
	})
}

func expectFtsProbe(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT gp_request_fts_probe_scan\(\);`).
		WillReturnRows(sqlmock.NewRows([]string{"gp_request_fts_probe_scan"}).AddRow("t"))
}

func expectGpSegmentConfigurationToReturn(mock sqlmock.Sqlmock, count int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM gp_segment_configuration 
WHERE content > -1 AND status = 'u' AND \(role = preferred_role\) AND mode = 's'`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectGpSegmentConfigurationWithoutMirrorsToReturn(mock sqlmock.Sqlmock, count int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM gp_segment_configuration 
WHERE content > -1 AND status = 'u' AND \(role = preferred_role\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectGpStatReplicationToReturn(mock sqlmock.Sqlmock, count int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM gp_stat_replication 
WHERE gp_segment_id = -1 AND state = 'streaming' AND sent_location = flush_location;`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func MustCreateCluster(t *testing.T, segs []greenplum.SegConfig) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segs)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
