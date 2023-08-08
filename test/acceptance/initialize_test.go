// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestInitialize(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("initialize accepts a port range", func(t *testing.T) {
		expectedPortRange := "30432,30434,30435,30436,30433,30437,30438,30439" // primaries,standby,mirrors

		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive", "--verbose",
			"--mode", idl.Mode_copy.String(),
			"--source-gphome", GPHOME_SOURCE,
			"--target-gphome", GPHOME_TARGET,
			"--source-master-port", PGPORT,
			"--temp-port-range", expectedPortRange,
			"--disk-free-ratio", "0")
		output, err := cmd.CombinedOutput()
		defer revert(t)
		if err != nil {
			t.Fatalf("unexpected err: %#v stderr %q", err, output)
		}

		conf, err := config.Read()
		if err != nil {
			t.Fatal(err)
		}

		var ports []string
		for _, seg := range conf.Intermediate.Primaries {
			ports = append(ports, strconv.Itoa(seg.Port))
		}

		for _, seg := range conf.Intermediate.Mirrors {
			ports = append(ports, strconv.Itoa(seg.Port))
		}

		expectedPorts := strings.Split(expectedPortRange, ",")

		sort.Strings(ports)
		sort.Strings(expectedPorts)

		if !reflect.DeepEqual(ports, expectedPorts) {
			t.Errorf("got %v want %v", ports, expectedPorts)
		}
	})
}
