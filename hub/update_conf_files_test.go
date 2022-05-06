// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

	pattern := `(^port[ \t]*=[ \t]*)%d([^0-9]|$)`
	replacement := `\1%d\2`

	t.Run("updates postgresql.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		standby := mock_idl.NewMockAgentClient(ctrl)
		standby.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:        "/data/standby/postgresql.conf",
					Pattern:     fmt.Sprintf(pattern, 50433),
					Replacement: fmt.Sprintf(replacement, 16432),
				}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:        "/data/dbfast_mirror2/seg2/postgresql.conf",
						Pattern:     fmt.Sprintf(pattern, 50436),
						Replacement: fmt.Sprintf(replacement, 25436),
					},
					{
						Path:        "/data/dbfast1/seg1/postgresql.conf",
						Pattern:     fmt.Sprintf(pattern, 50434),
						Replacement: fmt.Sprintf(replacement, 25433),
					}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:        "/data/dbfast_mirror1/seg1/postgresql.conf",
						Pattern:     fmt.Sprintf(pattern, 50434),
						Replacement: fmt.Sprintf(replacement, 25434),
					},
					{
						Path:        "/data/dbfast2/seg2/postgresql.conf",
						Pattern:     fmt.Sprintf(pattern, 50436),
						Replacement: fmt.Sprintf(replacement, 25435),
					}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

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
		standby.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:        "/data/standby/postgresql.conf",
					Pattern:     fmt.Sprintf(pattern, 50433),
					Replacement: fmt.Sprintf(replacement, 16432),
				}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateConfiguration(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateConfiguration(
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

func TestUpdateRecoveryConfOnSegments(t *testing.T) {
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

	pattern := `(primary_conninfo .* port[ \t]*=[ \t]*)%d([^0-9]|$)`
	replacement := `\1%d\2`

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
			standby.EXPECT().UpdateConfiguration(
				gomock.Any(),
				&idl.UpdateConfigurationRequest{
					Options: []*idl.UpdateFileConfOptions{{
						Path:        filepath.Join("/data/standby", c.file),
						Pattern:     fmt.Sprintf(pattern, 50432),
						Replacement: fmt.Sprintf(replacement, 15432),
					}},
				},
			).Return(&idl.UpdateConfigurationReply{}, nil)

			sdw1 := mock_idl.NewMockAgentClient(ctrl)
			sdw1.EXPECT().UpdateConfiguration(
				gomock.Any(),
				&idl.UpdateConfigurationRequest{
					Options: []*idl.UpdateFileConfOptions{
						{
							Path:        filepath.Join("/data/dbfast_mirror2/seg2", c.file),
							Pattern:     fmt.Sprintf(pattern, 50436),
							Replacement: fmt.Sprintf(replacement, 25435),
						}},
				},
			).Return(&idl.UpdateConfigurationReply{}, nil)

			sdw2 := mock_idl.NewMockAgentClient(ctrl)
			sdw2.EXPECT().UpdateConfiguration(
				gomock.Any(),
				&idl.UpdateConfigurationRequest{
					Options: []*idl.UpdateFileConfOptions{
						{
							Path:        filepath.Join("/data/dbfast_mirror1/seg1", c.file),
							Pattern:     fmt.Sprintf(pattern, 50434),
							Replacement: fmt.Sprintf(replacement, 25433),
						}},
				},
			).Return(&idl.UpdateConfigurationReply{}, nil)

			agentConns := []*idl.Connection{
				{AgentClient: standby, Hostname: "standby"},
				{AgentClient: sdw1, Hostname: "sdw1"},
				{AgentClient: sdw2, Hostname: "sdw2"},
			}

			err := hub.UpdateRecoveryConfOnSegments(agentConns, c.version, intermediate, target)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})
	}

	t.Run("returns errors when failing to update recovery.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		standby := mock_idl.NewMockAgentClient(ctrl)
		standby.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{{
					Path:        "/data/standby/recovery.conf",
					Pattern:     fmt.Sprintf(pattern, 50432),
					Replacement: fmt.Sprintf(replacement, 15432),
				}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		expected := errors.New("permission denied")
		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateConfiguration(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateConfiguration(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: standby, Hostname: "standby"},
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdateRecoveryConfOnSegments(agentConns, semver.MustParse("6.0.0"), intermediate, target)
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

func TestUpdateInternalAutoConfOnMirrors(t *testing.T) {
	intermediate := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "master", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	pattern := `(^gp_dbid=)%d([^0-9]|$)`
	replacement := `\1%d\2`

	t.Run("updates internal.auto.conf on mirrors excluding the standby", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:        "/data/dbfast_mirror2/seg.HqtFHX54y0o.2/internal.auto.conf",
						Pattern:     fmt.Sprintf(pattern, 5),
						Replacement: fmt.Sprintf(replacement, 6),
					}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:        "/data/dbfast_mirror1/seg.HqtFHX54y0o.1/internal.auto.conf",
						Pattern:     fmt.Sprintf(pattern, 3),
						Replacement: fmt.Sprintf(replacement, 4),
					}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdateInternalAutoConfOnMirrors(agentConns, intermediate)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}
	})

	t.Run("returns error when failing to update internal.auto.conf on segments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sdw1 := mock_idl.NewMockAgentClient(ctrl)
		sdw1.EXPECT().UpdateConfiguration(
			gomock.Any(),
			&idl.UpdateConfigurationRequest{
				Options: []*idl.UpdateFileConfOptions{
					{
						Path:        "/data/dbfast_mirror2/seg.HqtFHX54y0o.2/internal.auto.conf",
						Pattern:     fmt.Sprintf(pattern, 5),
						Replacement: fmt.Sprintf(replacement, 6),
					}},
			},
		).Return(&idl.UpdateConfigurationReply{}, nil)

		expected := errors.New("permission denied")
		sdw2 := mock_idl.NewMockAgentClient(ctrl)
		sdw2.EXPECT().UpdateConfiguration(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		agentConns := []*idl.Connection{
			{AgentClient: sdw1, Hostname: "sdw1"},
			{AgentClient: sdw2, Hostname: "sdw2"},
		}

		err := hub.UpdateInternalAutoConfOnMirrors(agentConns, intermediate)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestUpdateConfFiles(t *testing.T) {
	t.Run("UpdateGpperfmonConf", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		testutils.MustCreateDir(t, filepath.Join(dir, "gpperfmon", "conf"))
		path := filepath.Join(dir, "gpperfmon", "conf", "gpperfmon.conf")
		testutils.MustWriteToFile(t, path, `
log_location = /some/directory

# should not be replaced
other_log_location = /some/directory
`)

		err := hub.UpdateConfigurationFile([]*idl.UpdateFileConfOptions{{
			Path:        filepath.Join(dir, "gpperfmon", "conf", "gpperfmon.conf"),
			Pattern:     `^log_location = .*$`,
			Replacement: fmt.Sprintf("log_location = %s", filepath.Join(dir, "gpperfmon", "logs")),
		}})
		if err != nil {
			t.Errorf("UpdateGpperfmonConf() returned error %+v", err)
		}

		contents := testutils.MustReadFile(t, path)
		expected := fmt.Sprintf(`
log_location = %s

# should not be replaced
other_log_location = /some/directory
`, filepath.Join(dir, "gpperfmon", "logs"))
		if contents != expected {
			t.Errorf("replaced contents: %s\nwant: %s", contents, expected)
		}
	})

	t.Run("UpdatePostgresqlConf", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		path := filepath.Join(dir, "postgresql.conf")
		testutils.MustWriteToFile(t, path, `
port=5000
port=5000 # comment
port = 5000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`)

		err := hub.UpdateConfigurationFile([]*idl.UpdateFileConfOptions{{Path: path, Pattern: fmt.Sprintf(`(^port[ \t]*=[ \t]*)%d([^0-9]|$)`, 5000), Replacement: fmt.Sprintf(`\1%d\2`, 6000)}})
		if err != nil {
			t.Errorf("UpdatePostgresqlConf() returned error %+v", err)
		}

		contents := testutils.MustReadFile(t, path)
		expected := `
port=6000
port=6000 # comment
port = 6000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`
		if contents != expected {
			t.Errorf("replaced contents: %s\nwant: %s", contents, expected)
		}
	})
	t.Run("UpdateRecoveryConf", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		path := filepath.Join(dir, "recovery.conf")
		testutils.MustWriteToFile(t, path, `
standby_mode = 'on'
primary_conninfo = 'user=gpadmin host=sdw1 port=5000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'

# should not be replaced
#primary_conninfo = 'user=gpadmin host=sdw1 port=5000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
`)

		err := hub.UpdateConfigurationFile([]*idl.UpdateFileConfOptions{{Path: path, Pattern: fmt.Sprintf(`(primary_conninfo .* port[ \t]*=[ \t]*)%d([^0-9]|$)`, 5000), Replacement: fmt.Sprintf(`\1%d\2`, 6000)}})
		if err != nil {
			t.Errorf("UpdateRecoveryConf() returned error %+v", err)
		}

		contents := testutils.MustReadFile(t, path)
		expected := `
standby_mode = 'on'
primary_conninfo = 'user=gpadmin host=sdw1 port=6000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
primary_slot_name = 'internal_wal_replication_slot'

# should not be replaced
#primary_conninfo = 'user=gpadmin host=sdw1 port=6000 sslmode=disable sslcompression=1 krbsrvname=postgres application_name=gp_walreceiver'
`
		if contents != expected {
			t.Errorf("replaced contents: %s\nwant: %s", contents, expected)
		}
	})

	t.Run("returns errors", func(t *testing.T) {
		opts := []*idl.UpdateFileConfOptions{
			{
				Path:        "",
				Pattern:     "",
				Replacement: "",
			},
			{
				Path:        "",
				Pattern:     "",
				Replacement: "",
			}}

		err := hub.UpdateConfigurationFile(opts)
		var errs errorlist.Errors
		if !xerrors.As(err, &errs) {
			t.Fatalf("error %#v does not contain type %T", err, errs)
		}

		if len(errs) != len(opts) {
			t.Fatalf("got error count %d, want %d", len(errs), len(opts))
		}

		for _, err := range errs {
			expected := `update . using "/usr/bin/sed -E -i.bak s@@@ " failed with "sed:`
			if !strings.HasPrefix(err.Error(), expected) {
				t.Errorf("expected error to contain %q got %q", expected, err.Error())
			}
		}
	})
}
