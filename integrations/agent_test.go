// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"context"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestAgent(t *testing.T) {
	port := testutils.MustGetPort(t)

	cases := []struct {
		name     string
		expected int
		args     []string
	}{
		{
			name:     "agent starts on default port",
			expected: upgrade.DefaultAgentPort,
			args:     []string{"agent", "--daemonize"},
		},
		{
			name:     "agent starts on specified port",
			expected: port,
			args:     []string{"agent", "--daemonize", "--port", strconv.Itoa(port)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command("gpupgrade", c.args...)

			// in order to kill the daemonized process enable process groups
			cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
			output, err := cmd.CombinedOutput()
			defer func() {
				// To kill the daemonized process we need to kill all processes
				// in the group by sending a kill signal to -PGID. In this case,
				// since the PID is the same as the PGID we can send -PID.
				if err := unix.Kill(-cmd.Process.Pid, unix.SIGTERM); err != nil {
					t.Fatalf("failed to kill gpupgrade agent: %+v", err)
				}
			}()
			if err != nil {
				t.Errorf("gpupgrade agent returned unexpected error %+v", err)
				t.Logf("output:\n%s", string(output))
			}

			// connect to the specified port
			timeout := 1 * time.Second
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			address := "localhost" + ":" + strconv.Itoa(c.expected)
			_, err = grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				t.Fatalf("failed to connect to address %s %+v", address, err)
			}
		})
	}
}
