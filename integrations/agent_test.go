// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"context"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestAgent(t *testing.T) {
	cases := []struct {
		name string
		port int
	}{
		{
			name: "agent starts on default port",
			port: upgrade.DefaultAgentPort,
		},
		{
			name: "agent starts on specified port",
			port: testutils.MustGetPort(t),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command("gpupgrade", "agent", "--port", strconv.Itoa(c.port))
			err := cmd.Start()
			defer func() {
				if err := cmd.Process.Kill(); err != nil {
					t.Fatalf("failed to kill gpupgrade agent: %+v", err)
				}
			}()
			if err != nil {
				t.Errorf("gpupgrade agent returned unexpected error %+v", err)
			}

			// connect to the specified port
			timeout := 2 * time.Second
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			address := "localhost" + ":" + strconv.Itoa(c.port)
			conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				t.Fatalf("failed to connect to address %s %+v", address, err)
			}

			if conn.GetState() != connectivity.Ready {
				t.Errorf("got %s want Ready", conn.GetState())
			}
		})
	}
}
