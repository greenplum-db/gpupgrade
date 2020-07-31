// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"encoding/csv"
	"io"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

const tablespacesQuery = `
	SELECT
		fsedbid as dbid,
		upgrade_tablespace.oid as oid,
		spcname as name,
		case when is_user_defined_tablespace then location_with_oid else fselocation end as location,
		(is_user_defined_tablespace::int) as userdefined
	FROM (
			SELECT
				pg_tablespace.oid,
				*,
				(fselocation || '/' || pg_tablespace.oid) as location_with_oid,
				(spcname not in ('pg_default', 'pg_global'))  as is_user_defined_tablespace
			FROM pg_tablespace
			INNER JOIN pg_filespace_entry
			ON fsefsoid = spcfsoid
		) upgrade_tablespace`

// map<tablespaceOid, tablespaceInfo>
type SegmentTablespaces map[int]TablespaceInfo

// map<DbID, map<tablespaceOid, tablespaceInfo>>
type Tablespaces map[int]SegmentTablespaces

// slice of tablespace rows from database
type TablespaceTuples []Tablespace

// input file passed to pg_upgrade, it contains the tablespace information
// used by pg_upgrade to upgrade the segment tablespace
const TablespacesMappingFile = "tablespaces.txt"

type TablespaceInfo struct {
	Location    string
	UserDefined int
}

type Tablespace struct {
	DbId int
	Oid  int
	Name string
	*TablespaceInfo
}

func (t Tablespaces) GetMasterTablespaces() SegmentTablespaces {
	return t[MasterDbid]
}

func (t *TablespaceInfo) IsUserDefined() bool {
	return t.UserDefined == 1
}

func GetTablespaceLocationForDbId(t *idl.TablespaceInfo, dbId int) string {
	return filepath.Join(t.Location, strconv.Itoa(dbId))
}

func GetMasterTablespaceLocation(basePath string, oid int) string {
	return filepath.Join(basePath, strconv.Itoa(oid), strconv.Itoa(MasterDbid))
}

func GetTablespaceTuples(connection *dbconn.DBConn) (TablespaceTuples, error) {
	if !connection.Version.Is("5") {
		return nil, errors.New("version not supported to retrieve tablespace information")
	}

	results := make(TablespaceTuples, 0)
	err := connection.Select(&results, tablespacesQuery)
	if err != nil {
		return nil, xerrors.Errorf("tablespace query %q: %w", tablespacesQuery, err)
	}

	return results, nil
}

// convert the database tablespace query result to internal structure
func NewTablespaces(tuples TablespaceTuples) Tablespaces {
	clusterTablespaceMap := make(Tablespaces)
	for _, t := range tuples {
		tablespaceInfo := TablespaceInfo{Location: t.Location, UserDefined: t.UserDefined}
		if segTablespaceMap, ok := clusterTablespaceMap[t.DbId]; ok {
			segTablespaceMap[t.Oid] = tablespaceInfo
			clusterTablespaceMap[t.DbId] = segTablespaceMap
		} else {
			segTablespaceMap := make(SegmentTablespaces)
			segTablespaceMap[t.Oid] = tablespaceInfo
			clusterTablespaceMap[t.DbId] = segTablespaceMap
		}
	}

	return clusterTablespaceMap
}

// write the tuples returned from the database to a csv file
func (t TablespaceTuples) Write(w io.Writer) error {
	writer := csv.NewWriter(w)
	for _, tablespace := range t {
		line := []string{
			strconv.Itoa(tablespace.DbId),
			strconv.Itoa(tablespace.Oid),
			tablespace.Name,
			tablespace.Location,
			strconv.Itoa(tablespace.UserDefined)}
		if err := writer.Write(line); err != nil {
			return xerrors.Errorf("write record %q: %w", line, err)
		}
	}
	defer writer.Flush()
	return nil
}

// main function which does the following:
// 1. query the database to get tablespace information
// 2. write the tablespace information to a file
// 3. converts the tablespace information to an internal structure
func TablespacesFromDB(conn *dbconn.DBConn, tablespacesFile string) (Tablespaces, error) {
	if err := conn.Connect(1); err != nil {
		return nil, xerrors.Errorf("connect to cluster: %w", err)
	}
	defer conn.Close()

	tablespaceTuples, err := GetTablespaceTuples(conn)
	if err != nil {
		return nil, xerrors.Errorf("retrieve tablespace information: %w", err)
	}

	file, err := utils.System.Create(tablespacesFile)
	if err != nil {
		return nil, xerrors.Errorf("create tablespace file %q: %w", tablespacesFile, err)
	}
	defer file.Close()
	if err := tablespaceTuples.Write(file); err != nil {
		return nil, xerrors.Errorf("populate tablespace mapping file: %w", err)
	}

	return NewTablespaces(tablespaceTuples), nil
}
