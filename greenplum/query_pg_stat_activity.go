// Copyright (c) 2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
)

type StatActivity struct {
	Usename          string
	Application_name string
	Datname          string
	Query            string
}

type StatActivities []StatActivity

func (s StatActivities) Error() string {
	var b strings.Builder
	var t tabwriter.Writer
	t.Init(&b, 0, 0, 2, ' ', 0)

	for _, row := range s.table() {
		for _, col := range row {
			fmt.Fprintf(&t, "%s\t", col)
		}
		fmt.Fprintln(&t)
	}

	t.Flush()
	return b.String()
}

func (s StatActivities) table() [][]string {
	var rows [][]string
	for _, activity := range s {
		rows = append(rows, []string{activity.Application_name, activity.Usename, activity.Datname, activity.Query})
	}

	sort.Sort(utils.TableRows(rows))
	rows = append([][]string{{"Application:", "User:", "Database:", "Query:"}}, rows...)

	return rows
}

func QueryPgStatActivity(db *sql.DB, cluster *Cluster) error {
	query := `SELECT datname, usename, application_name, query FROM pg_stat_activity WHERE pid <> pg_backend_pid();`
	if cluster.Version.Major < 6 {
		query = `SELECT datname, usename, application_name, current_query FROM pg_stat_activity WHERE procpid <> pg_backend_pid();`
	}

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activities StatActivities
	for rows.Next() {
		var activity StatActivity
		err := rows.Scan(&activity.Datname, &activity.Usename, &activity.Application_name, &activity.Query)
		if err != nil {
			return xerrors.Errorf("pg_stat_activity: %w", err)
		}

		activities = append(activities, activity)
	}

	err = rows.Err()
	if err != nil {
		return err
	}

	if len(activities) > 0 {
		nextAction := "Please close all database connections before proceeding."
		return utils.NewNextActionErr(xerrors.Errorf(`Found %d active connections to the %s cluster.
MASTER_DATA_DIRECTORY=%s
PGPORT=%d

%s`, len(activities),
			cluster.Destination, cluster.CoordinatorDataDir(), cluster.CoordinatorPort(), activities), nextAction)
	}

	return nil
}
