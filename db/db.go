// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	_ "github.com/lib/pq" //_ import for the side effect of having postgres driver available
)

// TODO: do we need this code anymore?
func NewDBConn(masterHost string, masterPort int, dbname string) *dbconn.DBConn {
	currentUser, _, _ := utils.GetUser()
	username := utils.TryEnv("PGUSER", currentUser)
	if dbname == "" {
		dbname = utils.TryEnv("PGDATABASE", "")
	}
	hostname, _ := utils.GetHost()
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
