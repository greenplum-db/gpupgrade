// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"

	"github.com/blang/semver/v4"
	"golang.org/x/xerrors"
)

type SegConfig struct {
	DbID      int
	ContentID int
	Port      int
	Hostname  string
	DataDir   string
	Role      string
}

type SegConfigs []SegConfig

const (
	PrimaryRole = "p"
	MirrorRole  = "m"
)

func (s *SegConfig) IsMaster() bool {
	return s.ContentID == -1 && s.Role == PrimaryRole
}

func (s *SegConfig) IsStandby() bool {
	return s.ContentID == -1 && s.Role == MirrorRole
}

func (s *SegConfig) IsPrimary() bool {
	return s.ContentID != -1 && s.Role == PrimaryRole
}

func (s *SegConfig) IsMirror() bool {
	return s.ContentID != -1 && s.Role == MirrorRole
}

func (s *SegConfig) IsOnHost(hostname string) bool {
	return s.Hostname == hostname
}

func GetSegmentConfiguration(db *sql.DB, version semver.Version) ([]SegConfig, error) {
	query := `
SELECT
	dbid,
	content as contentid,
	port,
	hostname,
	datadir,
	role
FROM gp_segment_configuration
ORDER BY content, role;`

	if version.Major == 5 {
		query = `
SELECT
	s.dbid,
	s.content as contentid,
	s.port,
	s.hostname,
	e.fselocation as datadir,
	s.role
FROM gp_segment_configuration s
JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
JOIN pg_filespace f ON e.fsefsoid = f.oid
WHERE f.fsname = 'pg_system'
ORDER BY s.content, s.role;`
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SegConfig, 0)
	for rows.Next() {
		var seg SegConfig
		if err := rows.Scan(&seg.DbID, &seg.ContentID, &seg.Port, &seg.Hostname, &seg.DataDir, &seg.Role); err != nil {
			return nil, xerrors.Errorf("scanning gp_segment_configuration: %w", err)

		}

		results = append(results, seg)
	}

	if err := rows.Err(); err != nil {
		return nil, xerrors.Errorf("iterating gp_segment_configuration rows: %w", err)
	}

	return results, nil
}

// SelectSegmentConfigs returns a list of all segments that match the given selector
// function. Segments are visited in order of ascending content ID (primaries
// before mirrors).
func (s SegConfigs) Select(selector func(*SegConfig) bool) SegConfigs {
	var matches SegConfigs

	for _, seg := range s {
		if selector(&seg) {
			matches = append(matches, seg)
		}
	}

	return matches
}
