// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

// gpupgrade cluster jobs

type ClusterJob struct {
	Source, Target string
	OSVersion      string
}

type ClusterJobs []ClusterJob

func (c *ClusterJob) Name() string {
	return fmt.Sprintf("%s-to-%s-cluster-tests-%s", c.Source, c.Target, c.OSVersion)
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
		suffix = "-extensions"
	}

	return fmt.Sprintf("%s-to-%s%s", j.Source, j.Target, suffix)
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

type MultihostGpupgradeJob struct {
	Source, Target string
	OSVersion      string
}

func (j *MultihostGpupgradeJob) Name() string {
	return fmt.Sprintf("%s-%s", j.BaseName(), j.OSVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (j *MultihostGpupgradeJob) BaseName() string {
	return fmt.Sprintf("%s-to-%s-%s", j.Source, j.Target, "multihost-cluster-tests")
}

type MultihostClusterJobs []MultihostGpupgradeJob
