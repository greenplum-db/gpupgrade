// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/greenplum-db/gpupgrade/testutils/acceptance"
)

func TestGoVersion(t *testing.T) {
	t.Run("gpupgrade is compiled with the expected golang version from go.mod", func(t *testing.T) {
		// Since the go mod version contains the minimum required go version it
		// must be less than or equal to the compiled version.
		if !goModVersion(t).LTE(compiledVersion(t)) {
			t.Errorf("got %q want %q", goModVersion(t), compiledVersion(t))
		}
	})
}

func compiledVersion(t *testing.T) semver.Version {
	t.Helper()

	cmd := exec.Command("go", "version", filepath.Join(acceptance.MustGetRepoRoot(t), "gpupgrade"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected err: %#v stderr %q", err, output)
	}

	parts := strings.SplitN(strings.TrimSpace(string(output)), "gpupgrade: go", 2)
	return semver.MustParse(parts[1])
}

// goModVersion returns the minimum go version to compile gpupgrade
func goModVersion(t *testing.T) semver.Version {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(acceptance.MustGetRepoRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %#v", err)
	}

	var version semver.Version
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		prefix := "go "
		if strings.HasPrefix(line, prefix) {
			parts := strings.SplitN(line, prefix, 2)
			version, err = semver.ParseTolerant(parts[1])
			if err != nil {
				t.Fatalf("parse version: %#v", err)
			}

			break
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning go.mod: %#v", err)
	}

	return version
}
