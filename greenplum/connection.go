// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"

	_ "github.com/greenplum-db/gp-common-go-libs/dbconn" // used indirectly as the database driver
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	_ "github.com/jackc/pgx/v4" // used indirectly as the database driver
)

func (c *Cluster) Connection(options ...Option) string {
	opts := newOptionList(options...)

	port := c.CoordinatorPort()
	if opts.port > 0 {
		port = opts.port
	}

	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?search_path=", port)

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

	gplog.Debug("connecting to %s cluster using: %q", c.Destination, connURI)
	return connURI
}

type Option func(*optionList)

// Port defaults to coordinator port
func Port(port int) Option {
	return func(options *optionList) {
		options.port = port
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
