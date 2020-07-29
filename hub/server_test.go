// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/mock_agent"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

const timeout = 1 * time.Second

func TestHubStart(t *testing.T) {
	source := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p"},
	})

	target := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p"},
	})

	conf := &hub.Config{
		Source:                 source,
		Target:                 target,
		TargetInitializeConfig: hub.InitializeConfig{},
		Port:                   testutils.MustGetPort(t),
		AgentPort:              testutils.MustGetPort(t),
		UseLinkMode:            false,
		UpgradeID:              0,
	}

	t.Run("start correctly errors if stop is called first", func(t *testing.T) {
		h := hub.New(conf, grpc.DialContext, "")
		h.Stop(true)

		errChan := make(chan error, 1)
		go func() {
			errChan <- h.Start()
		}()

		select {
		case err := <-errChan:
			if !xerrors.Is(err, hub.ErrHubStopped) {
				t.Errorf("got error %#v want %#v", err, hub.ErrHubStopped)
			}
		case <-time.After(timeout): // use timeout to prevent test from hanging
			t.Error("timeout exceeded")
		}
	})

	t.Run("start returns an error when port is in use", func(t *testing.T) {
		portInUse, closeListener := mustListen(t)
		defer closeListener()

		conf.Port = portInUse
		h := hub.New(conf, grpc.DialContext, "")

		errChan := make(chan error, 1)
		go func() {
			errChan <- h.Start()
		}()

		select {
		case err := <-errChan:
			expected := "listen"
			if err != nil && !strings.Contains(err.Error(), expected) {
				t.Errorf("got error %#v want %#v", err, expected)
			}
		case <-time.After(timeout): // use timeout to prevent test from hanging
			t.Error("timeout exceeded")
		}
	})

	// This is inherently testing a race. It will give false successes instead
	// of false failures, so DO NOT ignore transient failures in this test!
	t.Run("will return from Start() if Stop is called concurrently", func(t *testing.T) {
		h := hub.New(conf, grpc.DialContext, "")

		readyChan := make(chan bool, 1)
		go func() {
			_ = h.Start()
			readyChan <- true
		}()

		h.Stop(true)

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

// TODO: These tests would be faster and more testable if we pass in a gRPC
//  dialer to AgentConns similar to how we test RestartAgents. Thus, we would be
//  able to use bufconn.Listen when creating a gRPC dialer. But since there
//  are many callers to AgentConns that is not an easy change.
func TestAgentConns(t *testing.T) {
	source := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
		{ContentID: -1, DbID: 2, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: "m"},
		{ContentID: 0, DbID: 3, Port: 25432, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		{ContentID: 0, DbID: 4, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast_mirror1/seg1", Role: "m"},
		{ContentID: 1, DbID: 5, Port: 25433, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: "p"},
		{ContentID: 1, DbID: 6, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast_mirror2/seg2", Role: "m"},
	})

	target := hub.MustCreateCluster(t, []greenplum.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "standby", DataDir: "/data/qddir/seg-1", Role: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "sdw1-mirror", DataDir: "/data/dbfast1/seg1", Role: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "sdw2-mirror", DataDir: "/data/dbfast2/seg2", Role: "p"},
	})

	agentServer, dialer, agentPort := mock_agent.NewMockAgentServer()
	defer agentServer.Stop()

	conf := &hub.Config{
		Source:                 source,
		Target:                 target,
		TargetInitializeConfig: hub.InitializeConfig{},
		Port:                   testutils.MustGetPort(t),
		AgentPort:              agentPort,
		UseLinkMode:            false,
		UpgradeID:              0,
	}

	testhelper.SetupTestLogger()

	t.Run("closes open connections when shutting down", func(t *testing.T) {
		h := hub.New(conf, dialer, "")

		go func() {
			_ = h.Start()
		}()

		// creating connections
		agentConns, err := h.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		ensureAgentConnsReachState(t, agentConns, connectivity.Ready)

		// closing connections
		h.Stop(true)
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		ensureAgentConnsReachState(t, agentConns, connectivity.Shutdown)
	})

	t.Run("retrieves the agent connections for the source cluster hosts excluding the master", func(t *testing.T) {
		h := hub.New(conf, dialer, "")

		go func() {
			_ = h.Start()
		}()

		agentConns, err := h.AgentConns()
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
		h := hub.New(conf, dialer, "")

		newConns, err := h.AgentConns()
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}

		savedConns, err := h.AgentConns()
		if err != nil {
			t.Fatalf("unexpected error: %#v", err)
		}

		if !reflect.DeepEqual(newConns, savedConns) {
			t.Errorf("got %v want %v", newConns, savedConns)
		}
	})

	// XXX This test takes 1.5 seconds because of EnsureConnsAreReady(...)
	t.Run("returns an error if any connections have non-ready states", func(t *testing.T) {
		h := hub.New(conf, dialer, "")

		agentConns, err := h.AgentConns()
		if err != nil {
			t.Errorf("unexpected error: %#v", err)
		}

		agentServer.Stop()

		ensureAgentConnsReachState(t, agentConns, connectivity.TransientFailure)

		_, err = h.AgentConns()
		expected := "the connections to the following hosts were not ready"
		if err != nil && !strings.Contains(err.Error(), expected) {
			t.Errorf("got error %#v want %#v", err, expected)
		}
	})

	t.Run("returns an error if any connections have non-ready states when first dialing", func(t *testing.T) {
		expected := errors.New("ahh!")
		errDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, expected
		}

		h := hub.New(conf, errDialer, "")

		_, err := h.AgentConns()
		if !xerrors.Is(err, expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}
	})
}

func ensureAgentConnsReachState(t *testing.T, agentConns []*hub.Connection, state connectivity.State) {
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

func TestHubSaveConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	conf := &hub.Config{
		Source:                 source,
		Target:                 target,
		TargetInitializeConfig: hub.InitializeConfig{},
		Port:                   12345,
		AgentPort:              54321,
		UseLinkMode:            false,
		UpgradeID:              0,
	}

	h := hub.New(conf, nil, "")

	t.Run("saves configuration contents to disk", func(t *testing.T) {
		stateDir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, stateDir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		// Write the hub's configuration.
		if err := h.SaveConfig(); err != nil {
			t.Errorf("SaveConfig returned error %+v", err)
		}

		// Reload the configuration and ensure the contents are the same.
		path := filepath.Join(stateDir, upgrade.ConfigFileName)
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("error opening config %q: %+v", path, err)
		}

		actual := new(hub.Config)
		if err := actual.Load(file); err != nil {
			t.Errorf("loading config: %+v", err)
		}

		if !reflect.DeepEqual(actual, h.Config) {
			t.Errorf("wrote config %#v want %#v", actual, h.Config)
		}
	})
}

func TestAgentHosts(t *testing.T) {
	cases := []struct {
		name     string
		cluster  *greenplum.Cluster
		expected []string // must be in alphabetical order
	}{{
		"master excluded",
		hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", Role: "p"},
			{ContentID: 0, Hostname: "sdw1", Role: "p"},
			{ContentID: 1, Hostname: "sdw1", Role: "p"},
		}),
		[]string{"sdw1"},
	}, {
		"master included if another segment is with it",
		hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", Role: "p"},
			{ContentID: 0, Hostname: "mdw", Role: "p"},
		}),
		[]string{"mdw"},
	}, {
		"mirror and standby hosts are handled",
		hub.MustCreateCluster(t, []greenplum.SegConfig{
			{ContentID: -1, Hostname: "mdw", Role: "p"},
			{ContentID: -1, Hostname: "smdw", Role: "m"},
			{ContentID: 0, Hostname: "sdw1", Role: "p"},
			{ContentID: 0, Hostname: "sdw1", Role: "m"},
			{ContentID: 1, Hostname: "sdw1", Role: "p"},
			{ContentID: 1, Hostname: "sdw2", Role: "m"},
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
