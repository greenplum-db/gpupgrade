// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/config"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/mock_agent"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

const timeout = 1 * time.Second

func TestHubStart(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	conf := &config.Config{
		Source:       source,
		Target:       target,
		Intermediate: &greenplum.Cluster{},
		HubPort:      testutils.MustGetPort(t),
		AgentPort:    testutils.MustGetPort(t),
		Mode:         idl.Mode_copy,
		UpgradeID:    0,
	}

	t.Run("start correctly errors if stop is called first", func(t *testing.T) {
		hubServer := hub.New(conf)
		hubServer.Stop(true)

		errChan := make(chan error, 1)
		go func() {
			errChan <- hubServer.Start(0, false)
		}()

		select {
		case err := <-errChan:
			if !errors.Is(err, hub.ErrHubStopped) {
				t.Errorf("got error %#v want %#v", err, hub.ErrHubStopped)
			}
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		}
	})

	t.Run("start returns an error when port is in use", func(t *testing.T) {
		portInUse, closeListener := mustListen(t)
		defer closeListener()

		conf.HubPort = portInUse
		hubServer := hub.New(conf)

		errChan := make(chan error, 1)
		go func() {
			errChan <- hubServer.Start(0, false)
		}()

		select {
		case err := <-errChan:
			expected := fmt.Sprintf("listen on port %d: listen tcp :%d: bind: address already in use", portInUse, portInUse)
			if err != nil && !strings.Contains(err.Error(), expected) {
				t.Errorf("got error %#v want %#v", err, expected)
			}
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		default:
			hubServer.Stop(false)
		}
	})

	// This is inherently testing a race. It will give false successes instead
	// of false failures, so DO NOT ignore transient failures in this test!
	t.Run("will return from Start() if Stop is called concurrently", func(t *testing.T) {
		hubServer := hub.New(conf)

		readyChan := make(chan bool, 1)
		go func() {
			_ = hubServer.Start(0, false)
			readyChan <- true
		}()

		hubServer.Stop(true)

		select {
		case isReady := <-readyChan:
			if !isReady {
				t.Errorf("expected start to return after calling stop")
			}
		case <-time.After(timeout): // use timeout to prevent test from hanging
			t.Error("timeout exceeded")
		}
	})
}

// getTcpListener returns a net.Listener and a function to close the listener
// for use in a defer.
func getTcpListener(t *testing.T) (net.Listener, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Errorf("unexpected error: %#v", err)
	}

	closeListener := func() {
		err := listener.Close()
		if err != nil {
			t.Fatalf("closing listener %#v", err)
		}
	}

	return listener, closeListener
}

func mustListen(t *testing.T) (int, func()) {
	t.Helper()

	listener, closeListener := getTcpListener(t)
	port := listener.Addr().(*net.TCPAddr).Port

	return port, closeListener
}

//	 TODO: These tests would be faster and more testable if we pass in a gRPC
//		dialer to AgentConns similar to how we test RestartAgents. Thus, we would be
//		able to use bufconn.Listen when creating a gRPC dialer. But since there
//		are many callers to AgentConns that is not an easy change.
func TestAgentConns(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: -1, DbID: 2, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: greenplum.MirrorRole},
		{ContentID: 0, DbID: 3, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 4, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{ContentID: 1, DbID: 5, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 6, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})

	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	agentServer, dialer, agentPort := mock_agent.NewMockAgentServer()
	defer agentServer.Stop()

	hub.SetgRPCDialer(dialer)
	defer hub.ResetgRPCDialer()

	conf := &config.Config{
		Source:       source,
		Target:       target,
		Intermediate: &greenplum.Cluster{},
		HubPort:      testutils.MustGetPort(t),
		AgentPort:    agentPort,
		Mode:         idl.Mode_copy,
		UpgradeID:    0,
	}

	testlog.SetupTestLogger()

	t.Run("closes open connections when shutting down", func(t *testing.T) {
		hubServer := hub.New(conf)

		go func() {
			_ = hubServer.Start(conf.HubPort, false)
		}()

		// creating connections
		agentConns, err := hubServer.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		ensureAgentConnsReachState(t, agentConns, connectivity.Ready)

		// closing connections
		hubServer.Stop(true)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		ensureAgentConnsReachState(t, agentConns, connectivity.Shutdown)
	})

	t.Run("retrieves the agent connections for the source cluster hosts excluding the coordinator", func(t *testing.T) {
		hubServer := hub.New(conf)

		go func() {
			_ = hubServer.Start(conf.HubPort, false)
		}()

		agentConns, err := hubServer.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		ensureAgentConnsReachState(t, agentConns, connectivity.Ready)

		var hosts []string
		for _, conn := range agentConns {
			hosts = append(hosts, conn.Hostname)
		}
		sort.Strings(hosts)

		expected := []string{"sdw1", "sdw1-mirror", "sdw2", "sdw2-mirror", "standby"}
		if !reflect.DeepEqual(hosts, expected) {
			t.Errorf("got %v want %v", hosts, expected)
		}
	})

	t.Run("saves grpc connections for future calls", func(t *testing.T) {
		hubServer := hub.New(conf)

		newConns, err := hubServer.AgentConns()
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}

		savedConns, err := hubServer.AgentConns()
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}

		if !reflect.DeepEqual(newConns, savedConns) {
			t.Errorf("got %v want %v", newConns, savedConns)
		}
	})

	t.Run("returns an error if any connections have non-ready states when first dialing", func(t *testing.T) {
		expected := errors.New("ahh!")
		hub.SetgRPCDialer(func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, expected
		})
		defer hub.ResetgRPCDialer()

		hubServer := hub.New(conf)

		_, err := hubServer.AgentConns()
		if !errors.Is(err, expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}
	})
}

func TestEnsureConnsAreReady(t *testing.T) {
	source := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: -1, DbID: 2, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: greenplum.MirrorRole},
		{ContentID: 0, DbID: 3, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 4, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast_mirror1/seg1", Role: greenplum.MirrorRole},
		{ContentID: 1, DbID: 5, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 6, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast_mirror2/seg2", Role: greenplum.MirrorRole},
	})

	target := hub.MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	agentServer, dialer, agentPort := mock_agent.NewMockAgentServer()
	defer agentServer.Stop()

	hub.SetgRPCDialer(dialer)
	defer hub.ResetgRPCDialer()

	conf := &config.Config{
		Source:       source,
		Target:       target,
		Intermediate: &greenplum.Cluster{},
		HubPort:      testutils.MustGetPort(t),
		AgentPort:    agentPort,
		Mode:         idl.Mode_copy,
		UpgradeID:    0,
	}

	testlog.SetupTestLogger()

	t.Run("succeeds when all agents are ready", func(t *testing.T) {
		hubServer := hub.New(conf)

		errChan := make(chan error, 1)
		go func() {
			errChan <- hubServer.Start(0, false)
		}()

		select {
		case err := <-errChan:
			t.Fatalf("unexpected error: %#v", err)
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		default:

		}

		agentConns, err := hubServer.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		err = hub.EnsureConnsAreReady(agentConns, 0*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}
	})

	t.Run("errors with all non-ready agent status when timeout is exceeded", func(t *testing.T) {
		hubServer := hub.New(conf)

		errChan := make(chan error, 1)
		go func() {
			errChan <- hubServer.Start(0, false)
		}()

		select {
		case err := <-errChan:
			t.Fatalf("unexpected error: %#v", err)
		case <-time.After(timeout):
			t.Error("timeout exceeded")
		default:

		}

		agentConns, err := hubServer.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		expected := hub.AgentsGrpcStatus{}
		for _, agentConn := range agentConns {
			if agentConn.Hostname == "sdw1" || agentConn.Hostname == "sdw2-mirror" {
				err := agentConn.Conn.Close()
				if err != nil {
					t.Fatalf("close mdw connection: %v", err)
				}

				expected[agentConn.Hostname] = agentConn.Conn.GetState()
			}
		}

		err = hub.EnsureConnsAreReady(agentConns, 0*time.Second)
		if !strings.HasSuffix(err.Error(), expected.String()) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func ensureAgentConnsReachState(t *testing.T, agentConns []*idl.Connection, state connectivity.State) {
	t.Helper()

	for _, conn := range agentConns {
		isReached, err := doesStateEventuallyReach(conn.Conn, state)
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}
		if !isReached {
			t.Error("expected connectivity state to be reached")
		}
	}
}

func doesStateEventuallyReach(conn *grpc.ClientConn, state connectivity.State) (bool, error) {
	startTime := time.Now()
	timeout := 3 * time.Second

	for {
		if conn.GetState() == state {
			return true, nil
		}

		if time.Since(startTime) > timeout {
			return false, xerrors.Errorf("timeout exceeded")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentHosts(t *testing.T) {
	cases := []struct {
		name     string
		cluster  *greenplum.Cluster
		expected []string // must be in alphabetical order
	}{{
		"coordinator excluded",
		hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "mdw", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "sdw1", Role: greenplum.PrimaryRole},
			{ContentID: 1, Hostname: "sdw1", Role: greenplum.PrimaryRole},
		}),
		[]string{"sdw1"},
	}, {
		"coordinator included if another segment is with it",
		hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "mdw", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "mdw", Role: greenplum.PrimaryRole},
		}),
		[]string{"mdw"},
	}, {
		"mirror and standby hosts are handled",
		hub.MustCreateCluster(t, greenplum.SegConfigs{
			{ContentID: -1, Hostname: "mdw", Role: greenplum.PrimaryRole},
			{ContentID: -1, Hostname: "smdw", Role: greenplum.MirrorRole},
			{ContentID: 0, Hostname: "sdw1", Role: greenplum.PrimaryRole},
			{ContentID: 0, Hostname: "sdw1", Role: greenplum.MirrorRole},
			{ContentID: 1, Hostname: "sdw1", Role: greenplum.PrimaryRole},
			{ContentID: 1, Hostname: "sdw2", Role: greenplum.MirrorRole},
		}),
		[]string{"sdw1", "sdw2", "smdw"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := hub.AgentHosts(c.cluster)
			sort.Strings(actual) // order not guaranteed

			if !reflect.DeepEqual(actual, c.expected) {
				t.Errorf("got %q want %q", actual, c.expected)
			}
		})
	}
}
