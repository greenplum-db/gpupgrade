// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func TestCheckDiskSpace(t *testing.T) {
	stateDir := testutils.GetTempDir(t, "")
	defer testutils.MustRemoveAll(t, stateDir)

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	t.Run("initialize fails when passed invalid --disk-free-ratio values", func(t *testing.T) {
		opts := []string{
			"1.5",
			"-0.5",
			"abcd",
		}

		for _, opt := range opts {
			cmd := exec.Command("gpupgrade", "initialize",
				"--non-interactive", "--verbose",
				"--source-gphome", GPHOME_SOURCE,
				"--target-gphome", GPHOME_TARGET,
				"--source-master-port", PGPORT,
				"--temp-port-range", TARGET_PGPORT+"-6040",
				"--stop-before-cluster-creation",
				"--disk-free-ratio", opt)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Errorf("expected nil got error %v", err)
			}

			initializeOutput := strings.ReplaceAll(string(output), `"`, ``)
			expected := fmt.Sprintf("Error: invalid argument %s for --disk-free-ratio flag", opt)
			if !strings.HasPrefix(initializeOutput, expected) {
				t.Fatalf("got %q want %q", initializeOutput, expected)
			}
		}
	})

	t.Run("initialize skips disk space check when --disk-free-ratio is 0", func(t *testing.T) {
		output := initialize(t, idl.Mode_copy)

		if strings.Contains(output, idl.Substep_check_disk_space.String()) {
			t.Fatalf("expected output %q to not contain %q", output, idl.Substep_check_disk_space)
		}
	})

	t.Run("initialize fails with disk space error", func(t *testing.T) {
		cmd := exec.Command("gpupgrade", "initialize",
			"--non-interactive", "--verbose",
			"--source-gphome", GPHOME_SOURCE,
			"--target-gphome", GPHOME_TARGET,
			"--source-master-port", PGPORT,
			"--temp-port-range", TARGET_PGPORT+"-6040",
			"--stop-before-cluster-creation",
			"--disk-free-ratio", "1.0")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected nil got error %v", err)
		}
		defer revert(t)

		expected := "You currently do not have enough disk space to run an upgrade"
		if !strings.Contains(string(output), expected) {
			t.Fatalf("expected %q to contain %q", string(output), expected)
		}

		actualUsages := actualDiskUsage(t, output)
		expectedUsages := expectedDiskUsage(t)

		for actualFS, actualUsage := range actualUsages {
			expectedUsage, ok := expectedUsages[actualFS]
			if !ok {
				t.Fatalf("actual filesystem not found. got %v want %v", actualFS, expectedUsage)
			}

			if !withinTolerance(actualUsage.GetAvailable(), expectedUsage.GetAvailable()) {
				t.Errorf("got %d want %d", actualUsage.GetAvailable(), expectedUsage.GetAvailable())
			}

			if !withinTolerance(actualUsage.GetRequired(), expectedUsage.GetRequired()) {
				t.Errorf("got %d want %d", actualUsage.GetRequired(), expectedUsage.GetRequired())
			}
		}
	})
}

func withinTolerance(actual uint64, expected uint64) bool {
	tolerance := 0.001

	percentDiff := math.Abs(float64(actual)-float64(expected)) / float64(expected)
	return percentDiff <= tolerance
}

func expectedDiskUsage(t *testing.T) map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage {
	source := GetSourceCluster(t)
	segments := source.SelectSegments(func(seg *greenplum.SegConfig) bool {
		return !seg.IsMirror()
	})

	usage := make(map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage)
	visitedHosts := make(map[string]bool)
	for _, seg := range segments {
		if _, ok := visitedHosts[seg.Hostname]; ok {
			continue
		}

		visitedHosts[seg.Hostname] = true

		fs := disk.FilesystemHost{Host: seg.Hostname, Filesystem: ""}
		usage[fs] = &idl.CheckDiskSpaceReply_DiskUsage{
			Fs:        "",
			Host:      seg.Hostname,
			Available: availableDiskSpaceInKb(t, seg.Hostname, seg.DataDir),
			Required:  totalDiskSpaceInKb(t, seg.Hostname, seg.DataDir), // required is total disk space since --disk-free-ratio is 1
		}
	}

	return usage
}

func availableDiskSpaceInKb(t *testing.T, host string, path string) uint64 {
	// Use the external stat utility rather than os.stat to test using a different
	// implementation than the code itself.
	cmd := exec.Command("ssh", host, GetStatUtility(), "-f -c '%S %a'", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	blockSize := testutils.MustConvertStringToInt(t, parts[0])
	blockNum := testutils.MustConvertStringToInt(t, parts[1])

	availableSpace := blockSize * blockNum / 1024
	return uint64(availableSpace)
}

func totalDiskSpaceInKb(t *testing.T, host string, path string) uint64 {
	// Use the external stat utility rather than os.stat to test using a different
	// implementation than the code itself.
	cmd := exec.Command("ssh", host, GetStatUtility(), "-f -c '%S %a %f %b'", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %v stderr: %q", err, output)
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	blockSize := testutils.MustConvertStringToInt(t, parts[0])
	blockAvailable := testutils.MustConvertStringToInt(t, parts[1])
	blockFree := testutils.MustConvertStringToInt(t, parts[2])
	blockTotal := testutils.MustConvertStringToInt(t, parts[3])

	totalSpace := (blockTotal - blockFree + blockAvailable) * blockSize / 1024
	return uint64(totalSpace)
}

func actualDiskUsage(t *testing.T, output []byte) map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage {
	usage := make(map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage)

	parseUsage := false
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Hostname") {
			parseUsage = true
			continue
		}

		if parseUsage && line != "" {
			parts := strings.Fields(line)

			fs := disk.FilesystemHost{Host: parts[0], Filesystem: ""}
			usage[fs] = &idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        parts[0],
				Host:      parts[1],
				Available: convertToKBytes(t, parts[4], parts[5]),
				Required:  convertToKBytes(t, parts[6], parts[7]),
			}

		}

		if parseUsage && line == "" {
			break
		}

	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning output: %v", err)
	}

	return usage
}

func convertToKBytes(t *testing.T, input string, units string) uint64 {
	num, err := strconv.ParseFloat(input, 64)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	switch strings.ToUpper(units) {
	case "KB":
		return uint64(num * math.Pow(1000, 0))
	case "MB":
		return uint64(num * math.Pow(1000, 1))
	case "GB":
		return uint64(num * math.Pow(1000, 2))
	case "TB":
		return uint64(num * math.Pow(1000, 3))
	case "PB":
		return uint64(num * math.Pow(1000, 4))
	case "EB":
		return uint64(num * math.Pow(1000, 5))
	default:
		t.Fatalf("Unit not found when converting disk usage output. Found %q", units)
	}

	return 0.0
}
