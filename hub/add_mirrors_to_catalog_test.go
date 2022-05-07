// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestAddMirrorsToCatalog(t *testing.T) {
	t.Run("adds all mirror segments excluding the standby", func(t *testing.T) {
		target := hub.MustCreateCluster(t, greenplum.SegConfigs{
			{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
			{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
			{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
			{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
			{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
			{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
		})

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer testutils.FinishMock(mock, t)
		defer db.Close()

		mock.MatchExpectationsInOrder(false) // since we iterate over maps for which golang does not guarantee order

		mock.ExpectBegin()

		for _, mirror := range target.Mirrors {
			if mirror.IsStandby() {
				continue
			}

			expectAddSegment(mock, mirror).WillReturnResult(sqlmock.NewResult(0, 1))
		}

		mock.ExpectCommit()

		err = hub.AddMirrorsToGpSegmentConfiguration(db, target)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}
	})

	// error cases
	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{DbID: 1, ContentID: -1, Hostname: "coordinator", DataDir: "/data/qddir/seg.HqtFHX54y0o.-1", Port: 50432, Role: greenplum.PrimaryRole},
		{DbID: 2, ContentID: -1, Hostname: "standby", DataDir: "/data/standby.HqtFHX54y0o", Port: 50433, Role: greenplum.MirrorRole},
		{DbID: 3, ContentID: 0, Hostname: "sdw1", DataDir: "/data/dbfast1/seg.HqtFHX54y0o.1", Port: 50434, Role: greenplum.PrimaryRole},
		{DbID: 4, ContentID: 0, Hostname: "sdw2", DataDir: "/data/dbfast_mirror1/seg.HqtFHX54y0o.1", Port: 50435, Role: greenplum.MirrorRole},
		{DbID: 5, ContentID: 1, Hostname: "sdw2", DataDir: "/data/dbfast2/seg.HqtFHX54y0o.2", Port: 50436, Role: greenplum.PrimaryRole},
		{DbID: 6, ContentID: 1, Hostname: "sdw1", DataDir: "/data/dbfast_mirror2/seg.HqtFHX54y0o.2", Port: 50437, Role: greenplum.MirrorRole},
	})

	expected := fmt.Errorf("sentinel error")
	ErrRollback := fmt.Errorf("rollback failed")

	errorCases := []struct {
		name          string
		expectations  func(sqlmock.Sqlmock)
		verifications func(*testing.T, error)
	}{{
		name: "errors when beginning a transaction fails",
		expectations: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin().WillReturnError(expected)
		},
		verifications: func(t *testing.T, actual error) {
			if !errors.Is(actual, expected) {
				t.Errorf("got %#v want %#v", actual, expected)
			}
		},
	}, {
		name: "rolls back transaction when inserting into the catalog fails",
		expectations: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO gp_segment_configuration").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO gp_segment_configuration").WillReturnError(expected)
			mock.ExpectRollback()
		},
		verifications: func(t *testing.T, actual error) {
			if !errors.Is(actual, expected) {
				t.Errorf("got %#v want %#v", actual, expected)
			}
		},
	}, {
		name: "errors when commit fails",
		expectations: func(mock sqlmock.Sqlmock) {
			mock.MatchExpectationsInOrder(false) // since we iterate over maps for which golang does not guarantee order
			mock.ExpectBegin()
			for _, mirror := range target.Mirrors.ExcludingStandby() {
				expectAddSegment(mock, mirror).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit().WillReturnError(expected)
		},
		verifications: func(t *testing.T, actual error) {
			if !errors.Is(actual, expected) {
				t.Errorf("got %#v want %#v", actual, expected)
			}
		},
	}, {
		name: "errors when rolling back fails",
		expectations: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO gp_segment_configuration").WillReturnError(expected)
			mock.ExpectRollback().WillReturnError(ErrRollback)
		},
		verifications: func(t *testing.T, err error) {
			var errs errorlist.Errors
			if !xerrors.As(err, &errs) {
				t.Fatalf("error %#v does not contain type %T", err, errs)
			}
			if !errors.Is(errs[0], expected) {
				t.Errorf("got %#v want %#v", err, expected)
			}
			if !errors.Is(errs[1], ErrRollback) {
				t.Errorf("got %#v want %#v", err, ErrRollback)
			}
		},
	}, {
		name: "rolls back if inserting into gp_segment_configuration returns multiple rows",
		expectations: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO gp_segment_configuration").WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectRollback()
		},
		verifications: func(t *testing.T, err error) {
			expected := "Expected 1 row to be added for segment dbid"
			if !strings.HasPrefix(err.Error(), expected) {
				t.Errorf("got %q want %q", err.Error(), expected)
			}
		},
	}}

	for _, c := range errorCases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer testutils.FinishMock(mock, t)
			defer db.Close()

			c.expectations(mock)
			err = hub.AddMirrorsToGpSegmentConfiguration(db, target)
			c.verifications(t, err)
		})
	}
}

func expectAddSegment(mock sqlmock.Sqlmock, seg greenplum.SegConfig) *sqlmock.ExpectedExec {
	return mock.ExpectExec("INSERT INTO gp_segment_configuration "+
		"\\(dbid, content, role, preferred_role, mode, status, port, hostname, address, datadir\\) "+
		"VALUES\\((.+), (.+), (.+), (.+), 'n', 'u', (.+), (.+), (.+), (.+)\\);").
		WithArgs(seg.DbID, seg.ContentID, seg.Role, seg.Role, seg.Port, seg.Hostname, seg.Hostname, seg.DataDir)
}
