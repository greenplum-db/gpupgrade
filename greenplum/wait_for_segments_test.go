// Copyright (c) 2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestWaitForSegments(t *testing.T) {
	timeout := 30 * time.Second

	target := MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
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
		target := MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
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
		target := MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
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
		target := MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
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
