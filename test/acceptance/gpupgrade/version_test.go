// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package gpupgrade_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "gpupgrade version prints version",
			args: []string{"version"},
		},
		{
			name: "gpupgrade --version prints version",
			args: []string{"--version"},
		},
		{
			name: "gpupgrade -V prints version",
			args: []string{"-V"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command("gpupgrade", c.args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("unexpected err: %#v stderr %q", err, output)
			}

			lines := strings.SplitN(strings.TrimSpace(string(output)), "\n", 3)

			expected := "Version: " + gitTag(t)
			if lines[0] != expected {
				t.Errorf("got %q want %q", lines[0], expected)
			}

			expected = "Commit: " + gitCommitSHA(t)
			if lines[1] != expected {
				t.Errorf("got %q want %q", lines[1], expected)
			}

			expected = "Release: Dev Build"
			if lines[2] != expected {
				t.Errorf("got %q want %q", lines[2], expected)
			}
		})
	}
}

func gitCommitSHA(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	return strings.TrimSpace(string(output))
}

func gitTag(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("unexpected err: %#v", err)
	}

	return strings.TrimSpace(string(output))
}
