// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
)

type Job struct {
	Source, Target string
	OSVersion      string
	Mode           Mode
	PrimariesOnly  bool
	NoStandby      bool
}

type Mode string

const (
	copy Mode = "copy"
	link Mode = "link"
)

type AcceptanceJob struct {
	Job
}

type AcceptanceJobs []AcceptanceJob

func (c *AcceptanceJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-acceptance-tests", c.Source, c.Target, c.OSVersion)
}

// upgrade jobs

type UpgradeJob struct {
	Job
	RetailDemo     bool
	TestExtensions bool
}

func (j *UpgradeJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-e2e-%s-mode%s", j.Source, j.Target, j.OSVersion, j.Mode, j.Suffix())
}

func (j *UpgradeJob) Suffix() string {
	var suffix string

	switch {
	case j.PrimariesOnly:
		suffix = "-primaries-only"
	case j.NoStandby:
		suffix = "-no-standby"
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

type PgUpgradeJob struct {
	Job
}

func (p *PgUpgradeJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-pg-upgrade-tests", p.Source, p.Target, p.OSVersion)
}

type PgUpgradeJobs []PgUpgradeJob

type MultihostAcceptanceJob struct {
	Job
}

func (j *MultihostAcceptanceJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-multihost-acceptance-tests", j.Source, j.Target, j.OSVersion)
}

type MultihostAcceptanceJobs []MultihostAcceptanceJob

type FunctionalJob struct {
	Job
	DumpPath string
}

func (j *FunctionalJob) Name() string {
	return fmt.Sprintf("%s-to-%s-%s-functional-test-%s-mode%s", j.Source, j.Target, j.OSVersion, j.Mode, j.Suffix())
}

func (j *FunctionalJob) Suffix() string {
	var suffix string

	switch {
	case j.PrimariesOnly:
		suffix = "-primaries-only"
	case j.NoStandby:
		suffix = "-no-standby"
	}

	return suffix
}

// SerialGroup is used to prevent Concourse from becoming overloaded.
func (j *FunctionalJob) SerialGroup() string {
	return strings.TrimPrefix(j.Suffix(), "-")
}

type FunctionalJobs []FunctionalJob
