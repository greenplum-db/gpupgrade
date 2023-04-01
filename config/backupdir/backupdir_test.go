// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package backupdir_test

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/greenplum-db/gpupgrade/config/backupdir"
	"github.com/greenplum-db/gpupgrade/greenplum"
)

func MustCreateCluster(t *testing.T, segments greenplum.SegConfigs) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segments)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
func TestParseParentBackupDirs(t *testing.T) {
	source := MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/coordinator/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby/seg-1", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data1/primaries/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data2/mirrors/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data2/primaries/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data1/mirrors/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})

	cases := []struct {
		name     string
		input    string
		expected backupdir.BackupDirs
	}{
		{
			name:  "defaults to the parent directory of the primary data directory on each host",
			input: "",
			expected: backupdir.BackupDirs{
				CoordinatorBackupDir: "/data/coordinator/.gpupgrade",
				AgentHostsToBackupDir: map[string]string{
					"sdw1": "/data1/primaries/.gpupgrade",
					"sdw2": "/data2/primaries/.gpupgrade",
				},
			},
		},
		{
			name:  "parses multiple hosts and directories",
			input: "cdw:/data/backup/coordinator,sdw1:/data1/backup/primaries,sdw2:/data2/backup/primaries",
			expected: backupdir.BackupDirs{
				CoordinatorBackupDir: "/data/backup/coordinator/.gpupgrade",
				AgentHostsToBackupDir: map[string]string{
					"sdw1": "/data1/backup/primaries/.gpupgrade",
					"sdw2": "/data2/backup/primaries/.gpupgrade",
				},
			},
		},
		{
			name:  "parses multiple hosts and directories with spaces",
			input: "   cdw:/data,   sdw1 : /data1 , sdw2 :  /data2   ",
			expected: backupdir.BackupDirs{
				CoordinatorBackupDir: "/data/.gpupgrade",
				AgentHostsToBackupDir: map[string]string{
					"sdw1": "/data1/.gpupgrade",
					"sdw2": "/data2/.gpupgrade",
				},
			},
		},
		{
			name:  "parses a single directory",
			input: "/data",
			expected: backupdir.BackupDirs{
				CoordinatorBackupDir: "/data/.gpupgrade",
				AgentHostsToBackupDir: map[string]string{
					"sdw1": "/data/.gpupgrade",
					"sdw2": "/data/.gpupgrade",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			backupDirs, err := backupdir.ParseParentBackupDirs(c.input, *source)
			if err != nil {
				t.Fatalf("unexpected error %#v", err)
			}

			if !reflect.DeepEqual(backupDirs, c.expected) {
				t.Errorf("got %v want %v", backupDirs, c.expected)
				t.Logf("got  %v", backupDirs)
				t.Logf("want %v", c.expected)
			}
		})
	}

	errorCases := []struct {
		name         string
		input        string
		missingHosts []string
	}{

		{
			name:         "errors when failing to specify a primary host host",
			input:        "cdw:/data/coordinator,sdw1:/data1/primaries",
			missingHosts: []string{"sdw2"},
		},
		{
			name:         "errors when failing to specify multiple hosts",
			input:        "cdw:/data/coordinator",
			missingHosts: []string{"sdw1", "sdw2"},
		},
	}

	for _, c := range errorCases {
		t.Run(c.name, func(t *testing.T) {
			backupDirs, err := backupdir.ParseParentBackupDirs(c.input, *source)
			if !reflect.DeepEqual(backupDirs, backupdir.BackupDirs{}) {
				t.Fatalf("expected backupDirs to be empty")
			}

			var missingHostInParentBackupDirsErr *backupdir.MissingHostInParentBackupDirsError
			if !errors.As(err, &missingHostInParentBackupDirsErr) {
				t.Fatalf("got %T, want %T", err, missingHostInParentBackupDirsErr)
			}

			if missingHostInParentBackupDirsErr.Input != c.input {
				t.Errorf("got input %q want %q", missingHostInParentBackupDirsErr.Input, c.input)
			}

			sort.Strings(missingHostInParentBackupDirsErr.MissingHosts)
			if !reflect.DeepEqual(missingHostInParentBackupDirsErr.MissingHosts, c.missingHosts) {
				t.Errorf("got missing hosts %q want %q", missingHostInParentBackupDirsErr.MissingHosts, c.missingHosts)
			}
		})
	}

}
