package services_test

import (
	"errors"

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
		agentA      *testutils.MockAgentServer
		port        int
		clusterPair *services.ClusterPair
	)

	BeforeEach(func() {
		agentA, port = testutils.NewMockAgentServer()
		clusterPair = &services.ClusterPair{
			OldCluster: testutils.CreateSampleCluster(-1, 25437, "localhost", "/old/datadir"),
		}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("closes open connections when shutting down", func(done Done) {
		defer close(done)
		hubConfig := &services.HubConfig{
			HubToAgentPort: port,
		}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, hubConfig, nil)
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
			HubToAgentPort: port,
		}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, hubConfig, nil)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.Ready))
		Expect(conns[0].Hostname).To(Equal("localhost"))
	})

	It("saves grpc connections for future calls", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: port,
		}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, hubConfig, nil)

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
			HubToAgentPort: port,
		}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, hubConfig, nil)

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
			HubToAgentPort: port,
		}

		hub := services.NewHub(clusterPair, mockDialer, nil, hubConfig, nil)

		_, err := hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

})
