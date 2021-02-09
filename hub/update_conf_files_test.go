// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
)

// TODO: this is an integration test; move it
func TestUpdateConfFiles(t *testing.T) {
	// Make execCommand and replacement "live" again
	hub.SetExecCommand(exec.Command)
	defer hub.ResetExecCommand()

	// This will be our "master data directory".
	dir, err := ioutil.TempDir("", "gpupgrade-unit-")
	if err != nil {
		t.Fatalf("creating temporary directory: %+v", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("removing temporary directory: %+v", err)
		}
	}()

	t.Run("UpdateGpperfmonConf", func(t *testing.T) {
		// Set up an example gpperfmon.conf.
		path := filepath.Join(dir, "gpperfmon", "conf", "gpperfmon.conf")
		writeFile(t, path, `
log_location = /some/directory

# should not be replaced
other_log_location = /some/directory
`)

		// Perform the replacement.
		err = hub.UpdateGpperfmonConf(step.DevNullStream, dir)
		if err != nil {
			t.Errorf("UpdateGpperfmonConf() returned error %+v", err)
		}

		// Check contents. The correct value depends on the temporary directory
		// location.
		logPath := filepath.Join(dir, "gpperfmon", "logs")
		expected := fmt.Sprintf(`
log_location = %s

# should not be replaced
other_log_location = /some/directory
`, logPath)

		checkContents(t, path, expected)
	})

	t.Run("UpdatePostgresqlConf", func(t *testing.T) {
		// Set up an example postgresql.conf.
		path := filepath.Join(dir, "postgresql.conf")
		writeFile(t, path, `
port=5000
port=5000 # comment
port = 5000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`)

		// Perform the replacement.
		err = hub.UpdatePostgresqlConf(step.DevNullStream, dir, 5000, 6000)
		if err != nil {
			t.Errorf("UpdatePostgresqlConf() returned error %+v", err)
		}

		checkContents(t, path, `
port=6000
port=6000 # comment
port = 6000 # make sure we can handle spaces

# should not be replaced
gpperfmon_port=5000
port=50000
#port=5000
`)
	})
}

func writeFile(t *testing.T, path string, contents string) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatalf("creating parent directory: %+v", err)
	}

	testutils.MustWriteToFile(t, path, contents)
}

func checkContents(t *testing.T, path string, expected string) {
	t.Helper()

	contents := testutils.MustReadFile(t, path)
	if contents != expected {
		t.Errorf("replaced contents: %s\nwant: %s", contents, expected)
	}
}
