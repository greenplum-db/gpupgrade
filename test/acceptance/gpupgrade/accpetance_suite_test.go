// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
)

var GPHOME_SOURCE string
var GPHOME_TARGET string
var PGPORT string

const TARGET_PGPORT = "6020"

func init() {
	gpHomeSource := os.Getenv("GPHOME_SOURCE")
	if gpHomeSource == "" {
		err := os.Setenv("GPHOME_SOURCE", testutils.MustGetEnv("GPHOME"))
		if err != nil {
			log.Fatalf("setting $GPHOME_SOURCE: %v", err)
		}
	}

	gpHomeTarget := os.Getenv("GPHOME_TARGET")
	if gpHomeTarget == "" {
		err := os.Setenv("GPHOME_TARGET", testutils.MustGetEnv("GPHOME"))
		if err != nil {
			log.Fatalf("setting $GPHOME_TARGET: %v", err)
		}
	}

	GPHOME_SOURCE = testutils.MustGetEnv("GPHOME_SOURCE")
	GPHOME_TARGET = testutils.MustGetEnv("GPHOME_TARGET")
	PGPORT = testutils.MustGetEnv("PGPORT")
}

func initialize(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "initialize",
		"--non-interactive",
		"--source-gphome", GPHOME_SOURCE,
		"--target-gphome", GPHOME_TARGET,
		"--source-master-port", PGPORT,
		"--temp-port-range", TARGET_PGPORT+"-6040",
		"--stop-before-cluster-creation",
		"--disk-free-ratio", "0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return string(output)
}

func killServices(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("gpupgrade", "kill-services")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	return string(output)
}
