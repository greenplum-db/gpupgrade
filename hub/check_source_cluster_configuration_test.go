// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestSegmentStatusError_Error(t *testing.T) {
	t.Run("it formats a list of dbids", func(t *testing.T) {
		err := hub.DownSegmentStatusError{DownDbids: []hub.DBID{1, 2, 3}}

		if !strings.Contains(err.Error(), "1, 2, 3") {
			t.Errorf("got %v, expected dbids to be included", err.Error())
		}
	})
}

func TestUnbalancedSegmentStatusError_Error(t *testing.T) {
	t.Run("it formats a list of dbids", func(t *testing.T) {
		err := hub.UnbalancedSegmentStatusError{UnbalancedDbids: []hub.DBID{1, 2, 3}}

		if !strings.Contains(err.Error(), "1, 2, 3") {
			t.Errorf("got %v, expected dbids to be included", err.Error())
		}
	})
}

func TestSegmentStatusErrors(t *testing.T) {
	t.Run("it passes when when all segments are up", func(t *testing.T) {
		sourceDatabase := []hub.SegmentStatus{
			{DbID: 0, IsUp: true},
			{DbID: 1, IsUp: true},
			{DbID: 2, IsUp: true},
		}

		err := hub.SegmentStatusErrors(sourceDatabase)
		if err != nil {
			t.Errorf("got unexpected error %+v", err)
		}
	})

	t.Run("it returns an error if any of the segments are not in their preferred role", func(t *testing.T) {
		sourceDatabase := []hub.SegmentStatus{
			makeBalanced(1),
			makeUnbalanced(2),
			makeBalanced(3),
			makeUnbalanced(4),
		}

		err := hub.SegmentStatusErrors(sourceDatabase)

		if err == nil {
			t.Fatalf("got no errors for step, expected segment status error")
		}

		var segmentStatusError hub.UnbalancedSegmentStatusError

		if !errors.As(err, &segmentStatusError) {
			t.Errorf("got an error that was not a segment status error: %v",
				err.Error())
		}

		unbalancedListIncludes := func(expectedDbid hub.DBID) bool {
			for _, dbid := range segmentStatusError.UnbalancedDbids {
				if dbid == expectedDbid {
					return true
				}
			}

			return false
		}

		if !unbalancedListIncludes(2) {
			t.Errorf("got unbalanced dbids of %v, expected list to include %v",
				segmentStatusError.UnbalancedDbids,
				2)
		}

		if !unbalancedListIncludes(4) {
			t.Errorf("got unbalanced dbids of %v, expected list to include %v",
				segmentStatusError.UnbalancedDbids,
				4)
		}

		if unbalancedListIncludes(1) {
			t.Errorf("got unbalanced dbids of %v, expected list NOT TO include %v",
				segmentStatusError.UnbalancedDbids,
				1)
		}

		if unbalancedListIncludes(3) {
			t.Errorf("got down dbids of %v, expected list NOT TO include %v",
				segmentStatusError.UnbalancedDbids,
				3)
		}

	})

	t.Run("it returns an error if any of the segments are down", func(t *testing.T) {
		sourceDatabase := []hub.SegmentStatus{
			{DbID: 0, IsUp: true},
			{DbID: 1, IsUp: false},
			{DbID: 2, IsUp: true},
			{DbID: 99, IsUp: false},
		}

		err := hub.SegmentStatusErrors(sourceDatabase)

		if err == nil {
			t.Fatalf("got no errors for step, expected segment status error")
		}

		var segmentStatusError hub.DownSegmentStatusError
		if !errors.As(err, &segmentStatusError) {
			t.Errorf("got an error that was not a segment status error: %v",
				err.Error())
		}

		downListIncludes := func(expectedDbid hub.DBID) bool {
			for _, dbid := range segmentStatusError.DownDbids {
				if dbid == expectedDbid {
					return true
				}
			}

			return false
		}

		if !downListIncludes(1) {
			t.Errorf("got down dbids of %v, expected list to include %v",
				segmentStatusError.DownDbids,
				1)
		}

		if !downListIncludes(99) {
			t.Errorf("got down dbids of %v, expected list to include %v",
				segmentStatusError.DownDbids,
				99)
		}

		if downListIncludes(0) {
			t.Errorf("got down dbids of %v, expected list NOT TO include %v",
				segmentStatusError.DownDbids,
				0)
		}

	})

	t.Run("it returns both unbalanced errors and down errors at the same time", func(t *testing.T) {
		sourceDatabase := []hub.SegmentStatus{
			{DbID: 1, IsUp: false},
			makeUnbalanced(2),
		}

		err := hub.SegmentStatusErrors(sourceDatabase)

		if err == nil {
			t.Fatalf("got no errors for step, expected segment status error")
		}

		var errs errorlist.Errors

		if !errors.As(err, &errs) {
			t.Fatalf("got error %#v, want type %T", err, errs)
		}

		var downSegmentStatusError hub.DownSegmentStatusError
		if !errors.As(errs[0], &downSegmentStatusError) {
			t.Errorf("got an error that was not a down segment status error: %v",
				err.Error())
		}

		var unbalancedSegmentStatusError hub.UnbalancedSegmentStatusError
		if !errors.As(errs[1], &unbalancedSegmentStatusError) {
			t.Errorf("got an error that was not an unbalanced segment status error: %v",
				err.Error())
		}
	})
}

func makeBalanced(dbid hub.DBID) hub.SegmentStatus {
	return hub.SegmentStatus{
		IsUp:          true,
		DbID:          dbid,
		Role:          hub.Primary,
		PreferredRole: hub.Primary,
	}
}

func makeUnbalanced(dbid hub.DBID) hub.SegmentStatus {
	return hub.SegmentStatus{
		IsUp:          true,
		DbID:          dbid,
		Role:          hub.Mirror,
		PreferredRole: hub.Primary,
	}
}

func TestCheckSourceClusterConfiguration(t *testing.T) {
	// Simple integration cases. We rely on the other units to give us corner
	// case coverage.
	t.Run("runs with happy path", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		rows := sqlmock.
			NewRows([]string{"dbid", "is_up", "role", "preferred_role"}).
			AddRow("1", true, hub.Primary, hub.Primary)
		sqlmock.ExpectQuery(".*").WillReturnRows(rows)

		err = hub.CheckSourceClusterConfiguration(connection)
		if err != nil {
			t.Errorf("got unexpected error %+v", err)
		}
	})

	t.Run("runs with angry path", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		expected := errors.New("sdkfjlsdfd")
		sqlmock.ExpectQuery(".*").WillReturnError(expected)

		err = hub.CheckSourceClusterConfiguration(connection)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestGetSegmentStatuses(t *testing.T) {
	t.Run("it returns segment statuses", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		rows := sqlmock.
			NewRows([]string{"dbid", "is_up", "role", "preferred_role"}).
			AddRow("1", true, hub.Mirror, hub.Primary).
			AddRow("2", false, hub.Primary, hub.Mirror)

		query := `select dbid, status = .* as is_up, role, preferred_role
			from gp_segment_configuration`

		sqlmock.ExpectQuery(query).
			WithArgs(hub.Up).
			WillReturnRows(rows)

		statuses, err := hub.GetSegmentStatuses(connection)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(statuses) != 2 {
			t.Fatalf("got %d rows, expected 2 rows to be returned", len(statuses))
		}

		first := statuses[0]
		if first.DbID != 1 || first.IsUp != true || first.Role != hub.Mirror || first.PreferredRole != hub.Primary {
			t.Errorf("segment status not populated correctly: %+v", first)
		}

		second := statuses[1]
		if second.DbID != 2 || second.IsUp != false || second.Role != hub.Primary || second.PreferredRole != hub.Mirror {
			t.Errorf("segment status not populated correctly: %+v", second)
		}
	})

	t.Run("it returns an error if it fails to query for statuses", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		expected := errors.New("ahhhh")
		sqlmock.ExpectQuery(".*").WillReturnError(expected)

		_, err = hub.GetSegmentStatuses(connection)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("it returns an error if a row scan fails", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		rows := sqlmock.
			NewRows([]string{"dbid", "is_up", "role", "preferred_role"}).
			AddRow("1", true, hub.Mirror, hub.Primary).
			AddRow("2", 2, hub.Primary, hub.Mirror)

		query := `select dbid, status = .* as is_up, role, preferred_role
			from gp_segment_configuration`

		expected := errors.New("sql: Scan error on column index 1, name \"is_up\": sql/driver: couldn't convert 2 into type bool")
		sqlmock.ExpectQuery(query).WithArgs(hub.Up).WillReturnRows(rows)

		_, err = hub.GetSegmentStatuses(connection)

		if err == nil || err.Error() != expected.Error() {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})

	t.Run("it returns an error if row iteration fails", func(t *testing.T) {
		connection, sqlmock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("creating sqlmock: %+v", err)
		}
		defer testutils.FinishMock(sqlmock, t)

		expected := errors.New("Next() failed")
		rows := sqlmock.
			NewRows([]string{"dbid", "is_up", "role", "preferred_role"}).
			AddRow("1", true, hub.Mirror, hub.Primary).
			RowError(0, expected)

		sqlmock.ExpectQuery(".*").WillReturnRows(rows)

		_, err = hub.GetSegmentStatuses(connection)
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}
