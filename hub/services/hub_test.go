package services_test

import (
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hub", func() {
	var (
		agentA             *testutils.MockAgentServer
		port               int
		stubRemoteExecutor *testutils.StubRemoteExecutor
		clusterPair        *services.ClusterPair
	)

	BeforeEach(func() {
		agentA, port = testutils.NewMockAgentServer()
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
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
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)
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

	It("retrieves the agent connections from the config file reader", func() {
		clusterPair.OldCluster.Segments[1] = cluster.SegConfig{Hostname: "localhost"}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.Ready))
		Expect(conns[0].Hostname).To(Equal("localhost"))
	})

	It("saves grpc connections for future calls", func() {
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)

		newConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(newConns).To(HaveLen(1))

		savedConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(savedConns).To(HaveLen(1))

		Expect(newConns[0]).To(Equal(savedConns[0]))
	})

	It("returns an error if any connections have non-ready states", func() {
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())
		Expect(conns).To(HaveLen(1))

		agentA.Stop()

		Eventually(func() connectivity.State { return conns[0].Conn.GetState() }).Should(Equal(connectivity.TransientFailure))

		_, err = hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any connections have non-ready states when first dialing", func() {
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)

		agentA.Stop()

		_, err := hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if the grpc dialer to the agent throws an error", func() {
		agentA.Stop()

		clusterPair.OldCluster.Segments[0] = cluster.SegConfig{Hostname: "example"}
		hub := services.NewHub(clusterPair, grpc.DialContext, nil, &services.HubConfig{
			HubToAgentPort: port,
		}, stubRemoteExecutor)

		_, err := hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})
})
