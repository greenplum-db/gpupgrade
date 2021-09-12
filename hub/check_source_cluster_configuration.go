// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const (
	Primary Role = "p"
	Mirror  Role = "m"
)

const (
	Up   Status = "u"
	Down Status = "d"
)

type DBID int
type Role string
type Status string

type SegmentStatus struct {
	IsUp          bool
	DbID          DBID
	Role          Role
	PreferredRole Role
}

func CheckSourceClusterConfiguration(db *sql.DB) error {
	statuses, err := GetSegmentStatuses(db)
	if err != nil {
		return err
	}

	return SegmentStatusErrors(statuses)
}

func GetSegmentStatuses(db *sql.DB) ([]SegmentStatus, error) {
	statuses := make([]SegmentStatus, 0)

	rows, err := db.Query(`
		select dbid, status = $1 as is_up, role, preferred_role
		from gp_segment_configuration
	`, Up)

	if err != nil {
		return nil, err
	}

	for rows.Next() {
		r := SegmentStatus{}
		err = rows.Scan(&r.DbID, &r.IsUp, &r.Role, &r.PreferredRole)
		if err != nil {
			return nil, err
		}

		statuses = append(statuses, r)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return statuses, err
}

func SegmentStatusErrors(statuses []SegmentStatus) error {
	var errs error

	if err := checkForDownSegments(statuses); err != nil {
		errs = errorlist.Append(errs, err)
	}

	if err := checkForUnbalancedSegments(statuses); err != nil {
		errs = errorlist.Append(errs, err)
	}

	return errs
}

func checkForDownSegments(statuses []SegmentStatus) error {
	downSegments := filterSegments(statuses, func(status SegmentStatus) bool {
		return !status.IsUp
	})

	if len(downSegments) > 0 {
		return NewDownSegmentStatusError(downSegments)
	}

	return nil
}

func checkForUnbalancedSegments(statuses []SegmentStatus) error {
	unbalancedSegments := filterSegments(statuses, func(status SegmentStatus) bool {
		return status.PreferredRole != status.Role
	})

	if len(unbalancedSegments) > 0 {
		return NewUnbalancedSegmentStatusError(unbalancedSegments)
	}

	return nil
}

func filterSegments(segments []SegmentStatus, filterMatches func(status SegmentStatus) bool) []SegmentStatus {
	var downSegments []SegmentStatus

	for _, segment := range segments {
		if filterMatches(segment) {
			downSegments = append(downSegments, segment)
		}
	}

	return downSegments
}

type UnbalancedSegmentStatusError struct {
	UnbalancedDbids []DBID
}

func (e UnbalancedSegmentStatusError) Error() string {
	var dbidStrings []string

	for _, dbid := range e.UnbalancedDbids {
		dbidStrings = append(dbidStrings, strconv.Itoa(int(dbid)))
	}

	message := fmt.Sprintf("Could not initialize gpupgrade. These"+
		" Greenplum segment dbids are not in their preferred role: %v."+
		" Run gprecoverseg -r to rebalance the cluster.", strings.Join(dbidStrings, ", "))

	return message
}

func NewUnbalancedSegmentStatusError(segments []SegmentStatus) error {
	var dbids []DBID

	for _, segment := range segments {
		dbids = append(dbids, segment.DbID)
	}

	return UnbalancedSegmentStatusError{dbids}
}

type DownSegmentStatusError struct {
	DownDbids []DBID
}

func (e DownSegmentStatusError) Error() string {
	var dbidStrings []string

	for _, dbid := range e.DownDbids {
		dbidStrings = append(dbidStrings, strconv.Itoa(int(dbid)))
	}

	message := fmt.Sprintf("Could not initialize gpupgrade. These"+
		" Greenplum segment dbids are not up: %v."+
		" Please bring all segments up before initializing.", strings.Join(dbidStrings, ", "))

	return message
}

func NewDownSegmentStatusError(downSegments []SegmentStatus) error {
	var downDbids []DBID

	for _, downSegment := range downSegments {
		downDbids = append(downDbids, downSegment.DbID)
	}

	return DownSegmentStatusError{downDbids}
}
