// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
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
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"text/template"
)

var versions = []Version{
	{
		Source:          "5",
		Target:          "6",
		OSVersion:       "centos6",
		OSVersionNumber: "6",
	},
	{
		Source:          "5",
		Target:          "6",
		OSVersion:       "centos7",
		OSVersionNumber: "7",
		SpecialJobs:     true, // To avoid exploding the test matrix set specialJobs for 5->6 for only a single OS.
	},
	{
		Source:          "6",
		Target:          "6",
		OSVersion:       "centos7", // To avoid exploding the test matrix have 6->6 for only a single OS.
		OSVersionNumber: "7",
	},
	//{
	//	Source:   "6",
	//	Target:   "7",
	//	OSVersion:       "rocky8",
	//	OSVersionNumber: "8",
	//	SpecialJobs:     true,
	//},
	//{
	//	Source:   "7",
	//	Target:   "7",
	//	OSVersion:       "rocky8",
	//	OSVersionNumber: "8",
	//},
}

type Data struct {
	JobType                 string
	BranchName              string
	MajorVersions           []string
	GPDBVersions            GPDBVersions
	AcceptanceJobs          AcceptanceJobs
	MultihostAcceptanceJobs MultihostAcceptanceJobs
	UpgradeJobs             UpgradeJobs
	PgupgradeJobs           PgUpgradeJobs
	FunctionalJobs          FunctionalJobs
}

var data Data

func init() {
	var majorVersions MajorVersions
	var gpdbVersions GPDBVersions
	var acceptanceJobs AcceptanceJobs
	var multihostAcceptanceJobs MultihostAcceptanceJobs
	var upgradeJobs UpgradeJobs
	var pgupgradeJobs PgUpgradeJobs
	var functionalJobs FunctionalJobs

	for _, version := range versions {
		if !majorVersions.contains(version.Source) {
			majorVersions = append(majorVersions, version.Source)
		}

		gpdbVersion := GPDBVersion{
			Version:          version,
			GPDBVersion:      version.Source,
			TestRCIdentifier: testRCIdentifier(version.Source),
		}

		if !gpdbVersions.contains(gpdbVersion) {
			gpdbVersions = append(gpdbVersions, gpdbVersion)
		}

		gpdbVersion = GPDBVersion{
			Version:          version,
			GPDBVersion:      version.Target, // need to add all combinations of version
			TestRCIdentifier: testRCIdentifier(version.Target),
		}

		if !gpdbVersions.contains(gpdbVersion) {
			gpdbVersions = append(gpdbVersions, gpdbVersion)
		}

		// To avoid too many duplicate acceptanceJobs have only one for different
		// major versions (ie: SpecialJobs), and only one for same major
		// versions (ie: 6-to-6 or 7-to-7).
		if version.SpecialJobs || (version.Source == version.Target) {
			acceptanceJobs = append(acceptanceJobs, AcceptanceJob{Job{Version: version}})
			multihostAcceptanceJobs = append(multihostAcceptanceJobs, MultihostAcceptanceJob{Job{Version: version}})
		}

		upgradeJobs = append(upgradeJobs, UpgradeJob{Job: Job{
			Version: version,
			Mode:    copy,
		}})

		if version.SpecialJobs {
			pgupgradeJobs = append(pgupgradeJobs, PgUpgradeJob{Job{Version: version}})
		}
	}

	specialUpgradeJobs := UpgradeJobs{
		UpgradeJob{Job: Job{PrimariesOnly: true}},
		UpgradeJob{Job: Job{NoStandby: true}},
		UpgradeJob{RetailDemo: true},
		UpgradeJob{TestExtensions: true},
	}

	// SpecialJobs cases for 5->6. (These are special-cased to avoid exploding the
	// test matrix too much.)
	for _, job := range specialUpgradeJobs {
		for _, version := range versions {
			if !version.SpecialJobs {
				continue
			}

			job.Version = version
			job.Mode = link
			upgradeJobs = append(upgradeJobs, job)
		}
	}

	specialFunctionalJobs := FunctionalJobs{
		FunctionalJob{Job: Job{Mode: link}, DumpPath: os.Getenv("DUMP_PATH")},
	}

	// SpecialJobs cases for 5->6. (These are special-cased to avoid exploding the
	// test matrix too much.)
	for _, job := range specialFunctionalJobs {
		for _, version := range versions {
			if !version.SpecialJobs {
				continue
			}

			job.Version = version
			job.Mode = link
			functionalJobs = append(functionalJobs, job)
		}
	}

	data = Data{
		JobType:                 os.Getenv("JOB_TYPE"),
		BranchName:              os.Getenv("BRANCH_NAME"),
		MajorVersions:           majorVersions,
		GPDBVersions:            gpdbVersions,
		AcceptanceJobs:          acceptanceJobs,
		MultihostAcceptanceJobs: multihostAcceptanceJobs,
		UpgradeJobs:             upgradeJobs,
		PgupgradeJobs:           pgupgradeJobs,
		FunctionalJobs:          functionalJobs,
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
