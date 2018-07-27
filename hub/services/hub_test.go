package services_test

import (
	"errors"
	"net"
	"strconv"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hub", func() {
	var (
		agentA         *testutils.MockAgentServer
		cliToHubPort   int
		hubToAgentPort int
		source         *utils.Cluster
		target         *utils.Cluster
		err            error
	)

	BeforeEach(func() {
		agentA, hubToAgentPort = testutils.NewMockAgentServer()
		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("will return from Start() with an error if Stop() is called first", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)

		hub.Stop()
		go func() {
			err = hub.Start()
		}()
		//Using Eventually ensures the test will not stall forever if this test fails.
		Eventually(func() error { return err }).Should(Equal(services.ErrHubStopped))
	})

	It("will return an error from Start() if it cannot listen on a port", func() {
		// Steal a port, and then try to start the hub on the same port.
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		defer listener.Close()

		_, portString, err := net.SplitHostPort(listener.Addr().String())
		Expect(err).NotTo(HaveOccurred())

		cliToHubPort, err := strconv.Atoi(portString)
		Expect(err).NotTo(HaveOccurred())

		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)

		go func() {
			err = hub.Start()
		}()
		//Using Eventually ensures the test will not stall forever if this test fails.
		Eventually(func() error { return err }).Should(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to listen"))
	})

	// This is inherently testing a race. It will give false successes instead
	// of false failures, so DO NOT ignore transient failures in this test!
	It("will return from Start() if Stop is called concurrently", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)
		done := make(chan bool, 1)

		go func() {
			hub.Start()
			done <- true
		}()
		hub.Stop()

		Eventually(done).Should(Receive())
	})

	It("closes open connections when shutting down", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)
		go hub.Start()

		By("creating connections")
		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.Ready))

		By("closing the connections")
		hub.Stop()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.Shutdown))
	})

	It("retrieves the agent connections for the hosts in the cluster", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.Ready))
		Expect(conns[0].Hostname).To(Equal("localhost"))
	})

	It("saves grpc connections for future calls", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)

		newConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(newConns).To(HaveLen(1))

		savedConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(savedConns).To(HaveLen(1))

		Expect(newConns[0]).To(Equal(savedConns[0]))
	})

	// XXX This test takes 1.5 seconds because of EnsureConnsAreReady(...)
	It("returns an error if any connections have non-ready states", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, grpc.DialContext, hubConfig, nil)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(conns).To(HaveLen(1))

		agentA.Stop()

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.TransientFailure))

		_, err = hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any connections have non-ready states when first dialing", func() {
		mockDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, errors.New("grpc dialer error")
		}

		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}

		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

		_, err := hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

})
