// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

// gpupgrade cluster jobs

type AcceptanceJob struct {
	Source, Target string
	OSVersion      string
}

type AcceptanceJobs []AcceptanceJob

func (c *AcceptanceJob) Name() string {
	return fmt.Sprintf("%s-to-%s-acceptance-tests-%s", c.Source, c.Target, c.OSVersion)
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
	return fmt.Sprintf("%s-%s", j.BaseName(), j.OSVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (j *UpgradeJob) BaseName() string {
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

	return fmt.Sprintf("%s-to-%s-e2e-%s", j.Source, j.Target, suffix)
}

type UpgradeJobs []UpgradeJob

// pgupgrade jobs

type PgUpgradeJob struct {
	Source, Target string
	OSVersion      string
}

func (p *PgUpgradeJob) Name() string {
	return fmt.Sprintf("%s-%s", p.BaseName(), p.OSVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (p *PgUpgradeJob) BaseName() string {
	return fmt.Sprintf("%s-to-%s-%s", p.Source, p.Target, "pg-upgrade-tests")
}

type PgUpgradeJobs []PgUpgradeJob

// multihost-gpupgrade jobs

type MultihostAcceptanceJob struct {
	Source, Target string
	OSVersion      string
}

func (j *MultihostAcceptanceJob) Name() string {
	return fmt.Sprintf("%s-%s", j.BaseName(), j.OSVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (j *MultihostAcceptanceJob) BaseName() string {
	return fmt.Sprintf("%s-to-%s-%s", j.Source, j.Target, "multihost-acceptance-tests")
}

type MultihostAcceptanceJobs []MultihostAcceptanceJob
