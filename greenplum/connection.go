// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v4"        // used indirectly as the database driver "pgx"
	_ "github.com/jackc/pgx/v4/stdlib" // used indirectly as the database driver "pgx"
)

func (c *Cluster) Connection(options ...Option) string {
	opts := newOptionList(options...)

	port := c.CoordinatorPort()
	if opts.port > 0 {
		port = opts.port
	}

	database := "template1"
	if opts.database != "" {
		database = opts.database
	}

	connURI := fmt.Sprintf("postgresql://localhost:%d/%s?search_path=", port, database)

	if opts.utilityMode {
		mode := "&gp_role=utility"
		if c.Version.Major < 7 {
			mode = "&gp_session_role=utility"
		}

		connURI += mode
	}

	if opts.allowSystemTableMods {
		connURI += "&allow_system_table_mods=true"
	}

	log.Printf("connecting to %s cluster with: %q", c.Destination, connURI)
	return connURI
}

type Option func(*optionList)

// Port defaults to coordinator port
func Port(port int) Option {
	return func(options *optionList) {
		options.port = port
	}
}

// Database defaults to template1
func Database(database string) Option {
	return func(options *optionList) {
		options.database = database
	}
}

func UtilityMode() Option {
	return func(options *optionList) {
		options.utilityMode = true
	}
}

func AllowSystemTableMods() Option {
	return func(options *optionList) {
		options.allowSystemTableMods = true
	}
}

type optionList struct {
	port                 int
	database             string
	utilityMode          bool
	allowSystemTableMods bool
}

func newOptionList(opts ...Option) *optionList {
	o := new(optionList)
	for _, option := range opts {
		option(o)
	}
	return o
}
