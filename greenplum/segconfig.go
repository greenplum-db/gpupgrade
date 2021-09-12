// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
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

func GetSegmentConfiguration(connection *dbconn.DBConn, version semver.Version) ([]SegConfig, error) {
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

	results := make([]SegConfig, 0)
	err := connection.Select(&results, query)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func MustGetSegmentConfiguration(connection *dbconn.DBConn) []SegConfig {
	segConfigs, err := GetSegmentConfiguration(connection, semver.Version{})
	gplog.FatalOnError(err)
	return segConfigs
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
