//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"

	"github.com/blang/semver/v4"
	_ "github.com/greenplum-db/gp-common-go-libs/dbconn" // used indirectly as the database driver
	_ "github.com/jackc/pgx/v4"                          // used indirectly as the database driver
)

// TODO: we should add the source/target ports here too, but they
//  are known after we need to first call this package.
type Conn struct {
	sourceVersion semver.Version
	targetVersion semver.Version
}

func Connection(sourceVersion semver.Version, targetVersion semver.Version) *Conn {
	conn := new(Conn)
	conn.sourceVersion = sourceVersion
	conn.targetVersion = targetVersion

	return conn
}

func (c *Conn) URI(options ...Option) string {
	opts := newOptionList(options...)

	version := c.sourceVersion
	if opts.connectToTarget {
		version = c.targetVersion
	}

	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?search_path=", opts.port)

	if opts.utilityMode {
		if version.LT(semver.MustParse("7.0.0")) {
			connURI += "&gp_session_role=utility"
		} else {
			connURI += "&gp_role=utility"
		}
	}

	if opts.allowSystemTableMods {
		connURI += "&allow_system_table_mods=true"
	}

	return connURI
}

type Option func(*optionList)

func ToSource() Option {
	return func(options *optionList) {
		options.connectToTarget = false
	}
}

func ToTarget() Option {
	return func(options *optionList) {
		options.connectToTarget = true
	}
}

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
	connectToTarget      bool
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
