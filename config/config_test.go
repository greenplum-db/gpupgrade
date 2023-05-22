// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/cli/commands"
	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func TestConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	conf := &config.Config{
		Source:       source,
		Target:       target,
		Intermediate: &greenplum.Cluster{},
		HubPort:      12345,
		AgentPort:    54321,
		Mode:         idl.Mode_copy,
		UpgradeID:    "ABC123",
	}

	t.Run("save configuration contents to disk and load it back", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		if err := conf.Write(); err != nil {
			t.Errorf("saving config: %+v", err)
		}

		actual, err := config.Read()
		if err != nil {
			t.Errorf("loading config: %+v", err)
		}

		if !reflect.DeepEqual(actual, conf) {
			t.Errorf("wrote config %#v but wanted %#v", actual, conf)
		}
	})
}

func TestCreate(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	greenplum.SetVersionCommand(exectest.NewCommand(PostgresGPVersion_5_29_10))
	defer greenplum.ResetVersionCommand()

	source := MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})
	source.GPHome = "/usr/local/source"
	targetGPHome := "/usr/local/target"

	const hubPort = 9999
	const agentPort = 8888
	const mode = idl.Mode_link
	const useHbaHostnames = false
	const parentBackupDirs = ""
	ports, err := commands.ParsePorts("50432-65535")
	if err != nil {
		t.Fatal(err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("couldn't create sqlmock: %v", err)
	}
	defer testutils.FinishMock(mock, t)

	t.Run("create is idempotent", func(t *testing.T) {
		var sourceOld os.FileInfo

		{ // creates initial cluster config files if none exist or fails"
			expectGpSegmentConfigurationToReturnCluster(mock, source)
			expectGpSegmentConfigurationCount(mock, source)
			expectPgStatReplicationToReturn(mock)
			expectPgTablespace(mock)

			conf, err := config.Create(db, hubPort, agentPort, source.GPHome, targetGPHome, mode, useHbaHostnames, ports, parentBackupDirs)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			err = conf.Write()
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}

			if reflect.DeepEqual(conf, config.Config{}) {
				t.Errorf("expected non empty config")
			}

			sourceOld, err = os.Stat(config.GetConfigFile())
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}
		}

		{ // creating cluster config files is idempotent
			expectGpSegmentConfigurationToReturnCluster(mock, source)
			expectGpSegmentConfigurationCount(mock, source)
			expectPgStatReplicationToReturn(mock)
			expectPgTablespace(mock)

			conf, err := config.Create(db, hubPort, agentPort, source.GPHome, targetGPHome, mode, useHbaHostnames, ports, parentBackupDirs)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if reflect.DeepEqual(conf, config.Config{}) {
				t.Errorf("expected non empty config")
			}

			var sourceNew os.FileInfo
			if sourceNew, err = os.Stat(config.GetConfigFile()); err != nil {
				t.Errorf("got unexpected error %#v", err)
			}

			if sourceOld.ModTime() != sourceNew.ModTime() {
				t.Errorf("want %#v got %#v", sourceOld.ModTime(), sourceNew.ModTime())
			}
		}

		{ // creating cluster config files succeeds on multiple runs
			expectGpSegmentConfigurationToReturnCluster(mock, source)
			expectGpSegmentConfigurationCount(mock, source)
			expectPgStatReplicationToReturn(mock)
			expectPgTablespace(mock)

			conf, err := config.Create(db, hubPort, agentPort, source.GPHome, targetGPHome, mode, useHbaHostnames, ports, parentBackupDirs)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if reflect.DeepEqual(conf, config.Config{}) {
				t.Errorf("expected non empty config")
			}
		}
	})

	t.Run("create adds known parameters including upgradeID", func(t *testing.T) {
		expectGpSegmentConfigurationToReturnCluster(mock, source)
		expectGpSegmentConfigurationCount(mock, source)
		expectPgStatReplicationToReturn(mock)
		expectPgTablespace(mock)

		conf, err := config.Create(db, hubPort, agentPort, source.GPHome, targetGPHome, mode, useHbaHostnames, ports, parentBackupDirs)
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		if conf.HubPort != hubPort {
			t.Errorf("got %d want %d", conf.HubPort, hubPort)
		}

		if conf.AgentPort != agentPort {
			t.Errorf("got %d want %d", conf.AgentPort, agentPort)
		}

		if conf.Source.GPHome != source.GPHome {
			t.Errorf("got %s want %s", conf.Source.GPHome, source.GPHome)
		}

		if conf.Source.CoordinatorPort() != source.CoordinatorPort() {
			t.Errorf("got %d want %d", conf.Source.CoordinatorPort(), source.CoordinatorPort())
		}

		version := semver.MustParse("5.29.10")
		if !conf.Source.Version.EQ(version) {
			t.Errorf("got %v, want %v", conf.Source.Version, version)
		}

		if conf.Target.GPHome != targetGPHome {
			t.Errorf("got %s want %s", conf.Target.GPHome, targetGPHome)
		}

		if conf.Intermediate.GPHome != targetGPHome {
			t.Errorf("got %s want %s", conf.Target.GPHome, targetGPHome)
		}

		if conf.Mode != mode {
			t.Errorf("got %s want %s", conf.Mode, mode)
		}

		if conf.UseHbaHostnames != useHbaHostnames {
			t.Errorf("got %t want %t", conf.UseHbaHostnames, useHbaHostnames)
		}

		if conf.UpgradeID == "" {
			t.Errorf("expected non-empty UpgradeID")
		}
	})
}

func expectGpSegmentConfigurationToReturnCluster(mock sqlmock.Sqlmock, cluster *greenplum.Cluster) {
	rows := sqlmock.NewRows([]string{"dbid", "contentid", "port", "hostname", "address", "datadir", "role"})
	for _, seg := range cluster.Primaries {
		rows.AddRow(seg.DbID, seg.ContentID, seg.Port, seg.Hostname, seg.Address, seg.DataDir, seg.Role)
	}

	for _, seg := range cluster.Mirrors {
		rows.AddRow(seg.DbID, seg.ContentID, seg.Port, seg.Hostname, seg.Address, seg.DataDir, seg.Role)
	}

	mock.ExpectQuery(`SELECT.*dbid.*FROM gp_segment_configuration`).
		WillReturnRows(rows)
}
func expectGpSegmentConfigurationCount(mock sqlmock.Sqlmock, cluster *greenplum.Cluster) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM gp_segment_configuration 
WHERE content > -1 AND status = 'u' AND \(role = preferred_role\) AND mode = 's'`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(len(cluster.ExcludingCoordinatorOrStandby())))
}

func expectPgTablespace(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .* FROM pg_tablespace`).
		WillReturnRows(sqlmock.NewRows([]string{"dbid", "oid", "name", "location", "userdefined"}))
}

func expectPgStatReplicationToReturn(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM pg_stat_replication
WHERE state = 'streaming' AND sent_location = flush_location;`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}
