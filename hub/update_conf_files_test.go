// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestUpdatePostgresqlConfOnSegments(t *testing.T) {
	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})

	t.Run("updates postgresql.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		standby := mock_idl.NewMockAgentClient(ctrl)
		standby.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			&idl.UpdatePostgresqlConfRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:         "/data/standby/postgresql.conf",
					CurrentValue: 50433,
					UpdatedValue: 16432,
				}},
			},
		).Return(&idl.UpdatePostgresqlConfReply{}, nil)

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			&idl.UpdatePostgresqlConfRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:         "/data/dbfast_mirror2/seg2/postgresql.conf",
						CurrentValue: 50436,
						UpdatedValue: 25436,
					},
					{
						Path:         "/data/dbfast1/seg1/postgresql.conf",
						CurrentValue: 50434,
						UpdatedValue: 25433,
					}},
			},
		).Return(&idl.UpdatePostgresqlConfReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			&idl.UpdatePostgresqlConfRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:         "/data/dbfast_mirror1/seg1/postgresql.conf",
						CurrentValue: 50434,
						UpdatedValue: 25434,
					},
					{
						Path:         "/data/dbfast2/seg2/postgresql.conf",
						CurrentValue: 50436,
						UpdatedValue: 25435,
					}},
			},
		).Return(&idl.UpdatePostgresqlConfReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: standby, Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdatePostgresqlConfOnSegments(agentConns, intermediate, target)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns errors when failing to update postgresql.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		standby := mock_idl.NewMockAgentClient(ctrl)
		standby.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			&idl.UpdatePostgresqlConfRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:         "/data/standby/postgresql.conf",
					CurrentValue: 50433,
					UpdatedValue: 16432,
				}},
			},
		).Return(&idl.UpdatePostgresqlConfReply{}, nil)

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdatePostgresqlConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: standby, Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdatePostgresqlConfOnSegments(agentConns, intermediate, target)
		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Fatalf("got error count %d, want %d", len(errs), 2)
		}

		for _, err := range errs {
			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		}
	})
}

func TestUpdateRecoveryConfiguration(t *testing.T) {
	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg-1", Port: 15432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby", Port: 16432, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Port: 25433, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg1", Port: 25434, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Port: 25435, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg2", Port: 25436, Role: greenplum.MirrorRole},
	})

	cases := []struct {
		name    string
		version semver.Version
		file    string
	}{
		{
			name:    "updates recovery.conf on segments when GPDB version is 6X",
			version: semver.MustParse("6.0.0"),
			file:    "recovery.conf",
		},
		{
			name:    "updates postgresql.auto.conf on segments when GPDB version is 7X or later",
			version: semver.MustParse("7.0.0"),
			file:    "postgresql.auto.conf",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			standby := mock_idl.NewMockAgentClient(ctrl)
			standby.EXPECT().UpdateRecoveryConf(
				gomock.Any(),
				&idl.UpdateRecoveryConfRequest{
					Options: []*idl.UpdateFileConfOptions{{
						Path:         filepath.Join("/data/standby", c.file),
						CurrentValue: 50432,
						UpdatedValue: 15432,
					}},
				},
			).Return(&idl.UpdateRecoveryConfReply{}, nil)

			sdw1 := mock_idl.NewMockAgentClient(ctrl)
			sdw1.EXPECT().UpdateRecoveryConf(
				gomock.Any(),
				&idl.UpdateRecoveryConfRequest{
					Options: []*idl.UpdateFileConfOptions{
						{
							Path:         filepath.Join("/data/dbfast_mirror2/seg2", c.file),
							CurrentValue: 50436,
							UpdatedValue: 25435,
						}},
				},
			).Return(&idl.UpdateRecoveryConfReply{}, nil)

			sdw2 := mock_idl.NewMockAgentClient(ctrl)
			sdw2.EXPECT().UpdateRecoveryConf(
				gomock.Any(),
				&idl.UpdateRecoveryConfRequest{
					Options: []*idl.UpdateFileConfOptions{
						{
							Path:         filepath.Join("/data/dbfast_mirror1/seg1", c.file),
							CurrentValue: 50434,
							UpdatedValue: 25433,
						}},
				},
			).Return(&idl.UpdateRecoveryConfReply{}, nil)

			agentConns := []*idl.Connection{
				{AgentClient: standby, Hostname: "standby"},
				{AgentClient: sdw1, Hostname: "sdw1"},
				{AgentClient: sdw2, Hostname: "sdw2"},
			}

			err := hub.UpdateRecoveryConfiguration(agentConns, c.version, intermediate, target)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})
	}

	t.Run("returns errors when failing to update recovery.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		standby := mock_idl.NewMockAgentClient(ctrl)
		standby.EXPECT().UpdateRecoveryConf(
			gomock.Any(),
			&idl.UpdateRecoveryConfRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:         "/data/standby/recovery.conf",
					CurrentValue: 50432,
					UpdatedValue: 15432,
				}},
			},
		).Return(&idl.UpdateRecoveryConfReply{}, nil)

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateRecoveryConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateRecoveryConf(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: standby, Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdateRecoveryConfiguration(agentConns, semver.MustParse("6.0.0"), intermediate, target)
		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != 2 {
			t.Fatalf("got error count %d, want %d", len(errs), 2)
		}

		for _, err := range errs {
			if !errors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", err, expected)
			}
		}
	})
}

// TODO: this is an integration test; move it
func TestUpdateConfFiles(t *testing.T) {
	// Make cmd and replacement "live" again
	hub.SetExecCommand(exec.Command)
	defer hub.ResetExecCommand()

	// This will be our "master data directory".
	dir, err := ioutil.TempDir("", "gpupgrade-unit-")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("removing temporary directory: %+v", err)
		}
	}()

	t.Run("UpdateGpperfmonConf", func(t *testing.T) {
		// Set up an example gpperfmon.conf.
		path := filepath.Join(dir, "gpperfmon", "conf", "gpperfmon.conf")
		writeFile(t, path, `
log_location = /some/directory

# should not be replaced
other_log_location = /some/directory
`)

		// Perform the replacement.
		err = hub.UpdateGpperfmonConf(dir)
		if err != nil {
			t.Errorf("UpdateGpperfmonConf() returned error %+v", err)
		}

		// Check contents. The correct value depends on the temporary directory
		// location.
		logPath := filepath.Join(dir, "gpperfmon", "logs")
		expected := fmt.Sprintf(`
log_location = %s

# should not be replaced
other_log_location = /some/directory
`, logPath)

		checkContents(t, path, expected)
	})

	t.Run("UpdatePostgresqlConf", func(t *testing.T) {
		// Set up an example postgresql.conf.
		path := filepath.Join(dir, "postgresql.conf")
		writeFile(t, path, `
port=5000
port=5000 # comment
port = 5000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`)

		// Perform the replacement.
		err = hub.UpdatePostgresqlConf(path, 5000, 6000)
		if err != nil {
			t.Errorf("UpdatePostgresqlConf() returned error %+v", err)
		}

		checkContents(t, path, `
port=6000
port=6000 # comment
port = 6000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`)
	})

	t.Run("UpdateRecoveryConf", func(t *testing.T) {
		// Set up an example recovery.conf.
		path := filepath.Join(dir, "recovery.conf")
		writeFile(t, path, `
standby_mode = 'on'
primary_conninfo = 'user=gpadmin host=sdw1 port=5000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'

# should not be replaced
#primary_conninfo = 'user=gpadmin host=sdw1 port=5000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
`)

		// Perform the replacement.
		err = hub.UpdateRecoveryConf(path, 5000, 6000)
		if err != nil {
			t.Errorf("UpdateRecoveryConf() returned error %+v", err)
		}

		checkContents(t, path, `
standby_mode = 'on'
primary_conninfo = 'user=gpadmin host=sdw1 port=6000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'

# should not be replaced
#primary_conninfo = 'user=gpadmin host=sdw1 port=6000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
`)
	})
}

func writeFile(t *testing.T, path string, contents string) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatalf("creating parent directory: %+v", err)
	}

	testutils.MustWriteToFile(t, path, contents)
}

func checkContents(t *testing.T, path string, expected string) {
	t.Helper()

	contents := testutils.MustReadFile(t, path)
	if contents != expected {
		t.Errorf("replaced contents: %s\nwant: %s", contents, expected)
	}
}
