// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
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
	"text/template"

	"github.com/blang/semver/v4"
)

var versions = []Version{
	{
		sourceVersion:   "5",
		targetVersion:   "6",
		osVersion:       "centos6",
		osVersionNumber: "6",
	},
	{
		sourceVersion:   "5",
		targetVersion:   "6",
		osVersion:       "centos7",
		osVersionNumber: "7",
		SpecialJobs:     true, // To avoid exploding the test matrix set specialJobs for 5->6 for only a single OS.
	},
	{
		sourceVersion:   "6",
		targetVersion:   "6",
		osVersion:       "centos7", // To avoid exploding the test matrix have 6->6 for only a single OS.
		osVersionNumber: "7",
	},
	//{
	//	sourceVersion:   "6",
	//	targetVersion:   "7",
	//	osVersion:       "rocky8",
	//	osVersionNumber: "8",
	//	SpecialJobs:     true,
	//},
	//{
	//	sourceVersion:   "7",
	//	targetVersion:   "7",
	//	osVersion:       "rocky8",
	//	osVersionNumber: "8",
	//},
}

type Data struct {
	JobType              string
	MajorVersions        []string
	GPDBVersions         GPDBVersions
	ClusterJobs          ClusterJobs
	MultihostClusterJobs MultihostClusterJobs
	UpgradeJobs          UpgradeJobs
	PgupgradeJobs        PgUpgradeJobs
}

var data Data

func init() {
	var majorVersions MajorVersions
	var gpdbVersions GPDBVersions
	var clusterJobs ClusterJobs
	var multihostClusterJobs MultihostClusterJobs
	var upgradeJobs UpgradeJobs
	var pgupgradeJobs PgUpgradeJobs

	for _, version := range versions {
		if !majorVersions.contains(version.sourceVersion) {
			majorVersions = append(majorVersions, version.sourceVersion)
		}

		gpdbVersion := GPDBVersion{
			OSVersion:        version.osVersion,
			OSVersionNumber:  version.osVersionNumber,
			GPDBVersion:      version.sourceVersion,
			TestRCIdentifier: version.testRCIdentifier(),
		}

		if !gpdbVersions.contains(gpdbVersion) {
			gpdbVersions = append(gpdbVersions, gpdbVersion)
		}

		gpdbVersion = GPDBVersion{
			OSVersion:        version.osVersion,
			OSVersionNumber:  version.osVersionNumber,
			GPDBVersion:      version.targetVersion, // need to add all combinations of version
			TestRCIdentifier: version.testRCIdentifier(),
		}

		if !gpdbVersions.contains(gpdbVersion) {
			gpdbVersions = append(gpdbVersions, gpdbVersion)
		}

		// To avoid too many duplicate clusterJobs have only one for different
		// major versions (ie: SpecialJobs), and only one for same major
		// versions (ie: 6-to-6 or 7-to-7).
		if version.SpecialJobs || (version.sourceVersion == version.targetVersion) {
			clusterJobs = append(clusterJobs, ClusterJob{
				Source:    version.sourceVersion,
				Target:    version.targetVersion,
				OSVersion: version.osVersion,
			})

			multihostClusterJobs = append(multihostClusterJobs, MultihostGpupgradeJob{
				Source:    version.sourceVersion,
				Target:    version.targetVersion,
				OSVersion: version.osVersion,
			})
		}

		upgradeJobs = append(upgradeJobs, UpgradeJob{
			Source:    version.sourceVersion,
			Target:    version.targetVersion,
			OSVersion: version.osVersion,
		})

		if version.SpecialJobs {
			pgupgradeJobs = append(pgupgradeJobs, PgUpgradeJob{
				Source:    version.sourceVersion,
				Target:    version.targetVersion,
				OSVersion: version.osVersion,
			})
		}
	}

	specialUpgradeJobs := UpgradeJobs{
		{LinkMode: true},
		{PrimariesOnly: true},
		{NoStandby: true},
		{RetailDemo: true},
		{TestExtensions: true},
	}

	// SpecialJobs cases for 5->6. (These are special-cased to avoid exploding the
	// test matrix too much.)
	for _, job := range specialUpgradeJobs {
		for _, version := range versions {
			if !version.SpecialJobs {
				continue
			}

			job.Source = version.sourceVersion
			job.Target = version.targetVersion
			job.OSVersion = version.osVersion

			upgradeJobs = append(upgradeJobs, job)
		}
	}

	data = Data{
		JobType:              os.Getenv("JOB_TYPE"),
		MajorVersions:        majorVersions,
		GPDBVersions:         gpdbVersions,
		ClusterJobs:          clusterJobs,
		MultihostClusterJobs: multihostClusterJobs,
		UpgradeJobs:          upgradeJobs,
		PgupgradeJobs:        pgupgradeJobs,
	}
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
