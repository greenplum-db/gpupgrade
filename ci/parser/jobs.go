// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
)

// gpupgrade cluster jobs

type AcceptanceJob struct {
	Source, Target string
	OSVersion      string
}

type AcceptanceJobs []AcceptanceJob

func (c *AcceptanceJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-acceptance-tests", c.Source, c.Target, c.OSVersion)
}

// upgrade jobs

type UpgradeJob struct {
	Source, Target string
	PrimariesOnly  bool
	NoStandby      bool
	LinkMode       bool
	RetailDemo     bool
	TestExtensions bool
	OSVersion      string
}

func (j *UpgradeJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-e2e%s", j.Source, j.Target, j.OSVersion, j.Suffix())
}

func (j *UpgradeJob) Suffix() string {
	var suffix string

	switch {
	case j.PrimariesOnly:
		suffix = "-primaries-only"
	case j.NoStandby:
		suffix = "-no-standby"
	case j.LinkMode:
		suffix = "-link-mode"
	case j.RetailDemo:
		suffix = "-retail-demo"
	case j.TestExtensions:
		suffix = "-extension"
	}

	return suffix
}

// SerialGroup is used to prevent Concourse from becoming overloaded.
func (j *UpgradeJob) SerialGroup() string {
	return strings.TrimPrefix(j.Suffix(), "-")
}

type UpgradeJobs []UpgradeJob

// pgupgrade jobs

type PgUpgradeJob struct {
	Source, Target string
	OSVersion      string
}

func (p *PgUpgradeJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-pg-upgrade-tests", p.Source, p.Target, p.OSVersion)
}

type PgUpgradeJobs []PgUpgradeJob

// multihost-gpupgrade jobs

type MultihostAcceptanceJob struct {
	Source, Target string
	OSVersion      string
}

func (j *MultihostAcceptanceJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-multihost-acceptance-tests", j.Source, j.Target, j.OSVersion)
}

type MultihostAcceptanceJobs []MultihostAcceptanceJob
