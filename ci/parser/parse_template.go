// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

/*
	This command is used to parse a template file using the text/template package.
	Given a list of source versions and target versions, it will render these
	versions into the places specified by the template.

	Usage:
	parse_template template.yml output.yml

	Note: This will overwrite the contents of output.yml (if the file already
	exists) with the parsed output.
*/
package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"text/template"

	"github.com/blang/semver/v4"
)

var sourceVersions = []string{"6", "5"}
var targetVersions = []string{"6"}
var centosVersions = []string{"6", "7"}

type GpupgradeJob struct {
	Source, Target string
}

func (j *GpupgradeJob) Name() string {
	return fmt.Sprintf("%s-to-%s-install-tests", j.Source, j.Target)
}

type UpgradeJob struct {
	Source, Target string
	PrimariesOnly  bool
	NoStandby      bool
	UseLinkMode    bool
	RetailDemo     bool
	CentosVersion  string
}

func (j *UpgradeJob) Name() string {
	return fmt.Sprintf("%s-centos-%s", j.BaseName(), j.CentosVersion)
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
	case j.UseLinkMode:
		suffix = "-link-mode"
	case j.RetailDemo:
		suffix = "-retail-demo"
	}

	return fmt.Sprintf("%s-to-%s%s", j.Source, j.Target, suffix)
}

type PgUpgradeJob struct {
	Source, Target string
	CentosVersion  string
}

func (p *PgUpgradeJob) Name() string {
	return fmt.Sprintf("%s-centos-%s", p.BaseName(), p.CentosVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (p *PgUpgradeJob) BaseName() string {
	return fmt.Sprintf("%s-to-%s-%s", p.Source, p.Target, "pg-upgrade-tests")
}

type MultihostBatsJob struct {
	Source, Target string
	CentosVersion  string
}

func (j *MultihostBatsJob) Name() string {
	return fmt.Sprintf("%s-centos-%s", j.BaseName(), j.CentosVersion)
}

// BaseName returns the pipeline job name without the operating system.
// This is used as a tag in Concourse's serial group to limit similar jobs
// between operating systems from running at once to avoid overloading Concourse.
func (j *MultihostBatsJob) BaseName() string {
	return fmt.Sprintf("%s-to-%s-%s", j.Source, j.Target, "multihost-bats")
}

type Version struct {
	CentosVersion string
	GPVersion     string
}

type Data struct {
	AllVersions       []string // combination of Source/Target
	UpgradeJobs       []*UpgradeJob
	LastTargetVersion string
	Versions          []*Version
	GpupgradeJobs     []*GpupgradeJob
	PgupgradeJobs     []*PgUpgradeJob
	MultihostBatsJobs []*MultihostBatsJob
}

var data Data

func init() {
	var checkJobs []*GpupgradeJob
	var upgradeJobs []*UpgradeJob
	var pgupgradeJobs []*PgUpgradeJob
	var multihostBatsJobs []*MultihostBatsJob
	for _, sourceVersion := range sourceVersions {
		for _, targetVersion := range targetVersions {
			checkJobs = append(checkJobs, &GpupgradeJob{
				Source: sourceVersion,
				Target: targetVersion,
			})
			for _, centosVersion := range centosVersions {
				upgradeJobs = append(upgradeJobs, &UpgradeJob{
					Source:        sourceVersion,
					Target:        targetVersion,
					CentosVersion: centosVersion,
				})

				// pgupgradeJobs are only needed for 5->6, 6->7, etc.
				if sourceVersion != targetVersion {
					pgupgradeJobs = append(pgupgradeJobs, &PgUpgradeJob{
						Source:        sourceVersion,
						Target:        targetVersion,
						CentosVersion: centosVersion,
					})
				}
				multihostBatsJobs = append(multihostBatsJobs, &MultihostBatsJob{
					Source:        sourceVersion,
					Target:        targetVersion,
					CentosVersion: centosVersion,
				})
			}
		}
	}

	var versions []*Version
	for _, sourceVersion := range sourceVersions {
		for _, centosVersion := range centosVersions {
			versions = append(versions, &Version{
				CentosVersion: centosVersion,
				GPVersion:     sourceVersion,
			})
		}
	}

	for _, centosVersion := range centosVersions {
		// Special cases for 5->6. (These are special-cased to avoid exploding the
		// test matrix too much.)
		special := []*UpgradeJob{
			{UseLinkMode: true},
			{PrimariesOnly: true},
			{NoStandby: true},
			{RetailDemo: true},
		}

		for _, job := range special {
			job.Source = "5"
			job.Target = "6"
			job.CentosVersion = centosVersion

			upgradeJobs = append(upgradeJobs, job)
		}
	}

	data = Data{
		AllVersions:       deduplicate(sourceVersions, targetVersions),
		UpgradeJobs:       upgradeJobs,
		LastTargetVersion: targetVersions[len(targetVersions)-1],
		Versions:          versions,
		GpupgradeJobs:     checkJobs,
		PgupgradeJobs:     pgupgradeJobs,
		MultihostBatsJobs: multihostBatsJobs,
	}
}

// deduplicate combines, deduplicates, and sorts two string slices.
func deduplicate(a, b []string) []string {
	var all []string

	all = append(all, a...)
	all = append(all, b...)

	set := make(map[string]bool)
	for _, value := range all {
		set[value] = true
	}

	var result []string
	for key := range set {
		result = append(result, key)
	}

	sort.Strings(result)
	return result
}

func main() {
	templateFilepath, pipelineFilepath := os.Args[1], os.Args[2]

	templateFuncs := template.FuncMap{
		// The escapeVersion function is used to ensure that the gcs-resource
		// concourse plugin regex matches the version correctly. As an example
		// if we didn't do this, 60100 would match version 6.1.0
		"escapeVersion": func(version string) string {
			return regexp.QuoteMeta(version)
		},

		// majorVersion parses its string as a semver and returns the major
		// component. E.g. "4.15.3" -> "4"
		"majorVersion": func(version string) string {
			v, err := semver.ParseTolerant(version)
			if err != nil {
				panic(err) // the template engine deals with panics nicely
			}

			return fmt.Sprintf("%d", v.Major)
		},
	}

	yamlTemplate, err := template.New("Pipeline Template").Funcs(templateFuncs).ParseFiles(templateFilepath)
	if err != nil {
		log.Fatalf("error parsing %s: %+v", templateFilepath, err)
	}
	// Duplicate version data here in order to simplify template logic

	templateFilename := filepath.Base(templateFilepath)
	// Create truncates the file if it already exists, and opens it for writing
	pipelineFile, err := os.Create(path.Join(pipelineFilepath))
	if err != nil {
		log.Fatalf("error opening %s: %+v", pipelineFilepath, err)
	}
	_, err = pipelineFile.WriteString("## Code generated by ci/generate.go - DO NOT EDIT\n")
	if err != nil {
		log.Fatalf("error writing %s: %+v", pipelineFilepath, err)
	}

	err = yamlTemplate.ExecuteTemplate(pipelineFile, templateFilename, data)
	closeErr := pipelineFile.Close()
	if err != nil {
		log.Fatalf("error executing template: %+v", err)
	}
	if closeErr != nil {
		log.Fatalf("error closing %s: %+v", pipelineFilepath, closeErr)
	}
}
