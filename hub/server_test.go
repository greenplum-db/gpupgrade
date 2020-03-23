package hub_test

import (
	"context"
	"net"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/hub"
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
		conf           *hub.Config
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
		conf = &hub.Config{source, target, hub.InitializeConfig{}, cliToHubPort, hubToAgentPort, useLinkMode, 0}
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

func TestHubSaveConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	useLinkMode := false
	conf := &hub.Config{source, target, hub.InitializeConfig{}, 12345, 54321, useLinkMode, 0}

	h := hub.New(conf, nil, "")

	t.Run("saves configuration contents to disk", func(t *testing.T) {
		// Set up utils.System.Create to return the write side of a pipe. We can
		// read from the other side to confirm what was saved to "disk".
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatalf("creating pipe: %+v", err)
		}
		defer func() {
			read.Close()
			write.Close()
		}()

		utils.System.Create = func(path string) (*os.File, error) {
			return write, nil
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		// Write the hub's configuration to the pipe.
		if err := h.SaveConfig(); err != nil {
			t.Errorf("SaveConfig() returned error %+v", err)
		}

		// Reload the configuration from the read side of the pipe and ensure the
		// contents are the same.
		actual := new(hub.Config)
		if err := actual.Load(read); err != nil {
			t.Errorf("loading configuration results: %+v", err)
		}

		if !reflect.DeepEqual(h.Config, actual) {
			t.Errorf("wrote config %#v, want %#v", actual, h.Config)
		}
	})

	t.Run("bubbles up file creation errors", func(t *testing.T) {
		expected := errors.New("can't create")

		utils.System.Create = func(path string) (*os.File, error) {
			return nil, expected
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := h.SaveConfig()
		if !xerrors.Is(err, expected) {
			t.Errorf("returned %#v, want %#v", err, expected)
		}
	})

	t.Run("bubbles up file manipulation errors", func(t *testing.T) {
		// A nil file will fail to write and close, so we can make sure things
		// are handled correctly.
		utils.System.Create = func(path string) (*os.File, error) {
			return nil, nil
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := h.SaveConfig()

		// multierror.Error that contains os.ErrInvalid is not itself an instance
		// of os.ErrInvalid, so unpack it to check existence of os.ErrInvalid
		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}

		for _, err := range merr.Errors {
			// For nil Files, operations return os.ErrInvalid.
			if !xerrors.Is(err, os.ErrInvalid) {
				t.Errorf("returned error %#v, want %#v", err, os.ErrInvalid)
			}
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
