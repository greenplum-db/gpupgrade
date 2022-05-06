// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"encoding/csv"
	"io"
	"path/filepath"
	"strconv"

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

type TablespaceInfo struct {
	Location    string
	UserDefined int
}

type Tablespace struct {
	DbId int
	Oid  int
	Name string
	Info TablespaceInfo
}

func (t Tablespaces) GetMasterTablespaces() SegmentTablespaces {
	return t[MasterDbid]
}

func (s SegmentTablespaces) UserDefinedTablespacesLocations() []string {
	var dirs []string
	for _, tsInfo := range s {
		if !tsInfo.IsUserDefined() {
			continue
		}

		dirs = append(dirs, tsInfo.Location)
	}

	return dirs
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

func GetTablespaceTuples(db *sql.DB) (TablespaceTuples, error) {
	rows, err := db.Query(tablespacesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]Tablespace, 0)
	for rows.Next() {
		var ts Tablespace
		if err := rows.Scan(&ts.DbId, &ts.Oid, &ts.Name, &ts.Info.Location, &ts.Info.UserDefined); err != nil {
			return nil, xerrors.Errorf("scanning pg_tablespace: %w", err)

		}

		results = append(results, ts)
	}

	if err := rows.Err(); err != nil {
		return nil, xerrors.Errorf("iterating pg_tablespace rows: %w", err)

	}

	return results, nil
}

// convert the database tablespace query result to internal structure
func NewTablespaces(tuples TablespaceTuples) Tablespaces {
	clusterTablespaceMap := make(Tablespaces)
	for _, t := range tuples {
		tablespaceInfo := TablespaceInfo{Location: t.Info.Location, UserDefined: t.Info.UserDefined}
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
			tablespace.Info.Location,
			strconv.Itoa(tablespace.Info.UserDefined)}
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
func TablespacesFromDB(db *sql.DB, tablespacesFile string) (Tablespaces, error) {
	tablespaceTuples, err := GetTablespaceTuples(db)
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
