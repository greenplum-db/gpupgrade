package hub_test

import (
	"context"
	"net"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/hub/state"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/mock_agent"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// msgStream is a mock server stream for InitializeStep().
type msgStream struct {
	LastStatus idl.Status
}

func (m *msgStream) Send(msg *idl.Message) error {
	m.LastStatus = msg.GetStatus().Status
	return nil
}

var _ = Describe("Hub", func() {
	var (
		agentA         *mock_agent.MockAgentServer
		cliToHubPort   int
		hubToAgentPort int
		source         *greenplum.Cluster
		target         *greenplum.Cluster
		conf           *state.Config
		err            error
		mockDialer     hub.Dialer
		useLinkMode    bool
	)

	BeforeEach(func() {
		agentA, mockDialer, hubToAgentPort = mock_agent.NewMockAgentServer()
		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
		source.Mirrors = map[int]greenplum.SegConfig{
			-1: {ContentID: -1, DbID: 1, Port: 15433, Hostname: "standby-host", DataDir: "/seg-1"},
			0:  {ContentID: 0, DbID: 2, Port: 25434, Hostname: "mirror-host1", DataDir: "/seg1"},
			1:  {ContentID: 1, DbID: 3, Port: 25435, Hostname: "mirror-host2", DataDir: "/seg2"},
		}
		useLinkMode = false
		conf = &state.Config{source, target, state.InitializeConfig{}, cliToHubPort, hubToAgentPort, useLinkMode}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("will return from Start() with an error if Stop() is called first", func() {
		h := hub.New(conf, mockDialer, "")

		h.Stop(true)
		go func() {
			err = h.Start()
		}()
		//Using Eventually ensures the test will not stall forever if this test fails.
		Eventually(func() error { return err }).Should(Equal(hub.ErrHubStopped))
	})

	It("will return an error from Start() if it cannot listen on a port", func() {
		// Steal a port, and then try to start the hub on the same port.
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		defer listener.Close()

		_, portString, err := net.SplitHostPort(listener.Addr().String())
		Expect(err).NotTo(HaveOccurred())

		conf.Port, err = strconv.Atoi(portString)
		Expect(err).NotTo(HaveOccurred())

		h := hub.New(conf, mockDialer, "")

		go func() {
			err = h.Start()
		}()
		//Using Eventually ensures the test will not stall forever if this test fails.
		Eventually(func() error { return err }).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to listen"))
	})

	// This is inherently testing a race. It will give false successes instead
	// of false failures, so DO NOT ignore transient failures in this test!
	It("will return from Start() if Stop is called concurrently", func() {
		h := hub.New(conf, mockDialer, "")
		done := make(chan bool, 1)

		go func() {
			h.Start()
			done <- true
		}()
		h.Stop(true)

		Eventually(done).Should(Receive())
	})

	It("closes open connections when shutting down", func() {
		h := hub.New(conf, mockDialer, "")
		go h.Start()

		By("creating connections")
		conns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Ready))
		}

		By("closing the connections")
		h.Stop(true)
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Shutdown))
		}
	})

	It("retrieves the agent connections for the hosts of non-master segments", func() {
		h := hub.New(conf, mockDialer, "")

		conns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Ready))
		}

		var allHosts []string
		for _, conn := range conns {
			allHosts = append(allHosts, conn.Hostname)
		}
		Expect(allHosts).To(ConsistOf([]string{
			"host1", "host2", "standby-host", "mirror-host1", "mirror-host2",
		}))
	})

	It("saves grpc connections for future calls", func() {
		h := hub.New(conf, mockDialer, "")

		newConns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		savedConns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Expect(newConns).To(ConsistOf(savedConns))
	})

	// XXX This test takes 1.5 seconds because of EnsureConnsAreReady(...)
	It("returns an error if any connections have non-ready states", func() {
		h := hub.New(conf, mockDialer, "")

		conns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		agentA.Stop()

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.TransientFailure))
		}

		_, err = h.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any connections have non-ready states when first dialing", func() {
		errDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, errors.New("grpc dialer error")
		}

		h := hub.New(conf, errDialer, "")

		_, err := h.AgentConns()
		Expect(err).To(HaveOccurred())
	})
})

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
