// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"

	"github.com/blang/semver/v4"
	_ "github.com/greenplum-db/gp-common-go-libs/dbconn" // used indirectly as the database driver
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	_ "github.com/jackc/pgx/v4" // used indirectly as the database driver
)

// TODO: we should add the source/target ports here too, but they
//  are known after we need to first call this package.
type Conn struct {
	SourceVersion semver.Version
	TargetVersion semver.Version
}

func Connection(sourceVersion semver.Version, targetVersion semver.Version) *Conn {
	conn := new(Conn)
	conn.SourceVersion = sourceVersion
	conn.TargetVersion = targetVersion

	return conn
}

func (c *Conn) URI(options ...Option) string {
	opts := newOptionList(options...)

	destination := "source"
	version := c.SourceVersion
	if opts.connectToTarget {
		destination = "target"
		version = c.TargetVersion
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

	gplog.Debug("connecting to %s cluster using: %q", destination, connURI)
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
