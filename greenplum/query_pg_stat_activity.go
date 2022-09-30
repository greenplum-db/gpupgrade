// Copyright (c) 2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"fmt"
	"strings"
	"text/tabwriter"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
)

type StatActivity struct {
	User             sql.NullString
	Application_name sql.NullString
	Datname          sql.NullString
	Query            sql.NullString
}

type StatActivities []StatActivity

func (s StatActivities) Error() string {
	var sb strings.Builder
	var tw tabwriter.Writer
	tw.Init(&sb, 0, 0, 1, ' ', 0)

	for _, activity := range s {
		fmt.Fprintf(&tw, "Application:\t%s\n", activity.Application_name.String)
		fmt.Fprintf(&tw, "User:\t%s\n", activity.User.String)
		fmt.Fprintf(&tw, "Database:\t%s\n", activity.Datname.String)
		fmt.Fprintf(&tw, "Query:\t%s\n", activity.Query.String)
		fmt.Fprintln(&tw)
	}

	tw.Flush()
	return sb.String()
}

func QueryPgStatActivity(db *sql.DB, cluster *Cluster) error {
	query := `SELECT application_name, usename, datname, query FROM pg_stat_activity WHERE pid <> pg_backend_pid() ORDER BY application_name, usename, datname;`
	if cluster.Version.Major < 6 {
		query = `SELECT application_name, usename, datname, current_query FROM pg_stat_activity WHERE procpid <> pg_backend_pid() ORDER BY application_name, usename, datname;`
	}

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var activities StatActivities
	for rows.Next() {
		var activity StatActivity
		err := rows.Scan(&activity.Application_name, &activity.User, &activity.Datname, &activity.Query)
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
