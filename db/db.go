// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"os/user"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	_ "github.com/lib/pq" //_ import for the side effect of having postgres driver available

	"github.com/greenplum-db/gpupgrade/utils"
)

// TODO: do we need this code anymore?
func NewDBConn(masterHost string, masterPort int, dbname string) *dbconn.DBConn {
	currentUser, err := utils.System.CurrentUser()
	if err != nil {
		gplog.Error("Failed to look up current user: %s", err)
		currentUser = &user.User{}
	}
	username := utils.TryEnv("PGUSER", currentUser.Username)

	if dbname == "" {
		dbname = utils.TryEnv("PGDATABASE", "")
	}

	hostname, err := utils.System.Hostname()
	if err != nil {
		gplog.Error("Failed to look up hostname: %s", err)
	}
	if masterHost == "" {
		masterHost = utils.TryEnv("PGHOST", hostname)
	}

	return &dbconn.DBConn{
		ConnPool: nil,
		NumConns: 0,
		Driver:   dbconn.GPDBDriver{},
		User:     username,
		DBName:   dbname,
		Host:     masterHost,
		Port:     masterPort,
		Tx:       nil,
		Version:  dbconn.GPDBVersion{},
	}
}
