// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestHasMirrors(t *testing.T) {
	cases := []struct {
		name     string
		cluster  *greenplum.Cluster
		expected bool
	}{
		{
			name: "returns true when cluster has mirrors and standby",
			cluster: MustCreateCluster(t, greenplum.SegConfigs{
				{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
				{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
				{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
				{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
			}),
			expected: true,
		},
		{
			name: "returns false when cluster has no mirrors and standby",
			cluster: MustCreateCluster(t, greenplum.SegConfigs{
				{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
				{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
			}),
			expected: false,
		},
		{
			name: "returns false when cluster has no mirrors and no standby",
			cluster: MustCreateCluster(t, greenplum.SegConfigs{
				{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
			}),
			expected: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := c.cluster.HasMirrors()
			if actual != c.expected {
				t.Errorf("got %t want %t", actual, c.expected)
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

		expected := greenplum.SegConfigs{
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

		expected := greenplum.SegConfigs{
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

		actualCluster, err := greenplum.ClusterFromDB(db, semver.MustParse("0.0.0"), "", idl.ClusterDestination_source)
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

		actualCluster, err := greenplum.ClusterFromDB(db, semver.MustParse("0.0.0"), "", idl.ClusterDestination_source)

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
		destination := idl.ClusterDestination_intermediate
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
	cluster := greenplum.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: 1, Role: greenplum.PrimaryRole},
		{ContentID: 2, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.MirrorRole},
	})

	// Ensure all segments are visited correctly.
	actual := cluster.SelectSegments(func(cluster *greenplum.SegConfig) bool {
		return true
	})
	sort.Sort(actual)

	expected := greenplum.SegConfigs{
		{ContentID: 1, Role: greenplum.PrimaryRole},
		{ContentID: 2, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.MirrorRole},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("SelectSegments(*) = %+v, want %+v", actual, expected)
	}

	// Test a simple selector.
	actual = cluster.SelectSegments(func(cluster *greenplum.SegConfig) bool {
		return cluster.ContentID > 1
	})
	sort.Sort(actual)

	expected = greenplum.SegConfigs{
		{ContentID: 2, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.MirrorRole},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("SelectSegments(ContentID > 1) = %+v, want %+v", actual, expected)
	}

}

func TestHasAllMirrorsAndStandby(t *testing.T) {
	t.Run("returns true on full cluster", func(t *testing.T) {
		segs := greenplum.SegConfigs{
			{ContentID: -1, Role: greenplum.PrimaryRole},
			{ContentID: -1, Role: greenplum.MirrorRole},
			{ContentID: 0, Role: greenplum.PrimaryRole},
			{ContentID: 0, Role: greenplum.MirrorRole},
			{ContentID: 1, Role: greenplum.PrimaryRole},
			{ContentID: 1, Role: greenplum.MirrorRole},
			{ContentID: 2, Role: greenplum.PrimaryRole},
			{ContentID: 2, Role: greenplum.MirrorRole},
		}
		cluster := greenplum.MustCreateCluster(t, segs)

		if !cluster.HasAllMirrorsAndStandby() {
			t.Errorf("expected a cluster that has all mirrors and a standby")
		}
	})

	cases := []struct {
		name string
		segs greenplum.SegConfigs
	}{
		{
			"returns false on cluster with no mirrors",
			greenplum.SegConfigs{
				{ContentID: -1, Role: greenplum.PrimaryRole},
				{ContentID: 0, Role: greenplum.PrimaryRole},
				{ContentID: 1, Role: greenplum.PrimaryRole},
				{ContentID: 2, Role: greenplum.PrimaryRole},
			},
		},
		{
			"returns false on cluster with mirrors but no standby",
			greenplum.SegConfigs{
				{ContentID: -1, Role: greenplum.PrimaryRole},
				{ContentID: 0, Role: greenplum.PrimaryRole},
				{ContentID: 0, Role: greenplum.MirrorRole},
				{ContentID: 1, Role: greenplum.PrimaryRole},
				{ContentID: 1, Role: greenplum.MirrorRole},
				{ContentID: 2, Role: greenplum.PrimaryRole},
				{ContentID: 2, Role: greenplum.MirrorRole},
			},
		},
		{
			"returns false on cluster with standby and no mirrors",
			greenplum.SegConfigs{
				{ContentID: -1, Role: greenplum.PrimaryRole},
				{ContentID: -1, Role: greenplum.MirrorRole},
				{ContentID: 0, Role: greenplum.PrimaryRole},
				{ContentID: 1, Role: greenplum.PrimaryRole},
				{ContentID: 2, Role: greenplum.PrimaryRole},
			},
		},
		{
			"returns false on cluster with only one mirror",
			greenplum.SegConfigs{
				{ContentID: -1, Role: greenplum.PrimaryRole},
				{ContentID: 0, Role: greenplum.PrimaryRole},
				{ContentID: 0, Role: greenplum.MirrorRole},
				{ContentID: 1, Role: greenplum.PrimaryRole},
				{ContentID: 2, Role: greenplum.PrimaryRole},
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

func TestRunGreenplumCmd(t *testing.T) {
	testlog.SetupLogger()

	cluster := MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
	})
	cluster.GPHome = "/usr/local/greenplum-db"

	t.Run("executes greenplum utility with greenplum_path.sh set and correct args", func(t *testing.T) {
		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "bash"
			if name != expected {
				t.Errorf("got %q want %q", name, expected)
			}

			expectedArgs := []string{"-c", "source /usr/local/greenplum-db/greenplum_path.sh && /usr/local/greenplum-db/bin/gpaddmirrors -a -i mirrors_config --hba-hostnames"}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("got %q want %q", args, expectedArgs)
			}
		})
		greenplum.SetGreenplumCommand(cmd)
		defer greenplum.ResetGreenplumCommand()

		err := cluster.RunGreenplumCmd(step.DevNullStream, "gpaddmirrors", "-a", "-i", "mirrors_config", "--hba-hostnames")
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("sets greenplum environment variables", func(t *testing.T) {
		coordinatorDataDirectory := "MASTER_DATA_DIRECTORY"
		resetEnv := testutils.MustClearEnv(t, coordinatorDataDirectory)
		defer resetEnv()

		pgPort := "PGPORT"
		resetEnv = testutils.MustClearEnv(t, pgPort)
		defer resetEnv()

		// Echo the environment to stdout and to a copy for debugging
		greenplum.SetGreenplumCommand(exectest.NewCommand(EnvironmentMain))
		defer greenplum.ResetGreenplumCommand()

		streams := &step.BufferedStreams{}
		err := cluster.RunGreenplumCmd(streams, "gpaddmirrors", "-a", "-i", "mirrors_config", "--hba-hostnames")
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		actual := streams.StdoutBuf.String()
		expected := "MASTER_DATA_DIRECTORY=/data/qddir/seg-1\nPGPORT=15432\n"
		if actual != expected {
			t.Errorf("got %q want %q", actual, expected)
		}
	})

	t.Run("returns errors", func(t *testing.T) {
		greenplum.SetGreenplumCommand(exectest.NewCommand(FailedMain))
		defer greenplum.ResetGreenplumCommand()

		err := cluster.RunGreenplumCmd(step.DevNullStream, "gpaddmirrors", "-a", "-i", "mirrors_config", "--hba-hostnames")
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Errorf("got %T, want %T", err, exitError)
		}
	})
}

func MustCreateCluster(t *testing.T, segments greenplum.SegConfigs) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segments)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
