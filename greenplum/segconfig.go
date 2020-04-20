// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
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

func GetSegmentConfiguration(connection *dbconn.DBConn) ([]SegConfig, error) {
	query := ""
	if connection.Version.Before("6") {
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
ORDER BY s.content;`
	} else {
		query = `
SELECT
	dbid,
	content as contentid,
	port,
	hostname,
	datadir,
	role
FROM gp_segment_configuration
ORDER BY content;`
	}

	results := make([]SegConfig, 0)
	err := connection.Select(&results, query)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func MustGetSegmentConfiguration(connection *dbconn.DBConn) []SegConfig {
	segConfigs, err := GetSegmentConfiguration(connection)
	gplog.FatalOnError(err)
	return segConfigs
}
