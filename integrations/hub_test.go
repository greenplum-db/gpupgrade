// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestHub(t *testing.T) {
	port := testutils.MustGetPort(t)

	cases := []struct {
		name           string
		expected       int
		args           []string
		configContents string
	}{
		{
			name:           "starts on default port when flag is not set and config file is empty",
			expected:       upgrade.DefaultHubPort,
			args:           []string{"hub", "--daemonize"},
			configContents: `{}`,
		},
		{
			name:           "starts on flag value when flag is set and config file is empty",
			expected:       port,
			args:           []string{"hub", "--daemonize", "--port", strconv.Itoa(port)},
			configContents: `{}`,
		},
		{
			// command line arguments take precedence over config values
			name:           "starts on flag value when flag is set and config file contains a value",
			expected:       port,
			args:           []string{"hub", "--daemonize", "--port", strconv.Itoa(port)},
			configContents: `{"Port": 80}`,
		},
		{
			name:           "starts on config value when flag is not set and config file contains a value",
			expected:       port,
			args:           []string{"hub", "--daemonize"},
			configContents: fmt.Sprintf(`{"Port": %d}`, port),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := testutils.GetTempDir(t, "")
			defer testutils.MustRemoveAll(t, dir)

			resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", filepath.Join(dir, ".gpupgrade"))
			defer resetEnv()

			err := commanders.CreateStateDir()
			if err != nil {
				t.Errorf("CreateStateDir returned error: %+v", err)
			}

			// write initial config.json
			testutils.MustWriteToFile(t, upgrade.GetConfigFile(), c.configContents)

			cmd := exec.Command("gpupgrade", c.args...)

			// in order to kill the daemonized process enable process groups
			cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
			output, err := cmd.CombinedOutput()
			defer func() {
				// To kill the daemonized process we need to kill all processes
				// in the group by sending a kill signal to -PGID. In this case,
				// since the PID is the same as the PGID we can send -PID.
				if err := unix.Kill(-cmd.Process.Pid, unix.SIGTERM); err != nil {
					t.Fatalf("failed to kill gpupgrade hub: %+v", err)
				}
			}()
			if err != nil {
				t.Errorf("gpupgrade hub returned unexpected error %+v", err)
				t.Logf("output:\n%s", string(output))
			}

			// connect to the specified port
			timeout := 1 * time.Second
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			address := "localhost" + ":" + strconv.Itoa(c.expected)
			_, err = grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				t.Fatalf("failed to connect to address %s: %+v", address, err)
			}
		})
	}

	t.Run("the hub does not daemonizes unless --deamonize is passed", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", filepath.Join(dir, ".gpupgrade"))
		defer resetEnv()

		err := commanders.CreateStateDir()
		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		err = commanders.CreateConfigFile(upgrade.DefaultHubPort)
		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		cmd := exec.Command("gpupgrade", "hub")
		err = cmd.Start()
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
		defer func() {
			if err := cmd.Process.Kill(); err != nil {
				t.Fatalf("failed to kill gpupgrade hub: %+v", err)
			}
		}()

		errChan := make(chan error, 1)
		go func() {
			errChan <- cmd.Wait() // expected to never return
		}()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("unexpected error %+v", err)
			}
		case <-time.After(100 * time.Millisecond):
			// hub daemonizes without an error
		}
	})
}
