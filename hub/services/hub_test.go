package services_test

import (
	"net"
	"strconv"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

// msgStream is a mock server stream for InitializeStep().
type msgStream struct {
	LastStatus idl.StepStatus
}

func (m *msgStream) Send(msg *idl.Message) error {
	m.LastStatus = msg.GetStatus().Status
	return nil
}

var _ = Describe("Hub", func() {
	var (
		agentA         *testutils.MockAgentServer
		cliToHubPort   int
		hubToAgentPort int
		source         *utils.Cluster
		target         *utils.Cluster
		err            error
		mockDialer     services.Dialer
		mockStream     *msgStream
	)

	BeforeEach(func() {
		agentA, mockDialer, hubToAgentPort = testutils.NewMockAgentServer()
		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
		mockStream = &msgStream{}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("will return from Start() with an error if Stop() is called first", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

		hub.Stop(true)
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
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

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
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)
		done := make(chan bool, 1)

		go func() {
			hub.Start()
			done <- true
		}()
		hub.Stop(true)

		Eventually(done).Should(Receive())
	})

	It("closes open connections when shutting down", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)
		go hub.Start()

		By("creating connections")
		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Ready))
		}

		By("closing the connections")
		hub.Stop(true)
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Shutdown))
		}
	})

	It("retrieves the agent connections for the hosts of non-master segments", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.Ready))
		}

		var allHosts []string
		for _, conn := range conns {
			allHosts = append(allHosts, conn.Hostname)
		}
		Expect(allHosts).To(ConsistOf([]string{"host1", "host2"}))
	})

	It("saves grpc connections for future calls", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

		newConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		savedConns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Expect(newConns).To(ConsistOf(savedConns))
	})

	// XXX This test takes 1.5 seconds because of EnsureConnsAreReady(...)
	It("returns an error if any connections have non-ready states", func() {
		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}
		hub := services.NewHub(source, target, mockDialer, hubConfig, nil)

		conns, err := hub.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		agentA.Stop()

		for _, conn := range conns {
			Eventually(func() connectivity.State { return conn.Conn.GetState() }).Should(Equal(connectivity.TransientFailure))
		}

		_, err = hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error if any connections have non-ready states when first dialing", func() {
		errDialer := func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, errors.New("grpc dialer error")
		}

		hubConfig := &services.HubConfig{
			HubToAgentPort: hubToAgentPort,
		}

		hub := services.NewHub(source, target, errDialer, hubConfig, nil)

		_, err := hub.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("successfully initializes step by marking it as in-progress with status running", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)
		hub.InitializeStep("dub-step", mockStream)

		Expect(mockChecklistManager.GetStepReader("dub-step").Status()).To(Equal(idl.StepStatus_RUNNING))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("returns an error when InitializeStep fails to reset state directory", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.ResetStateDirErr = errors.New("permission denied")

		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)
		_, err := hub.InitializeStep("dub-step", mockStream)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("failed to reset state directory: permission denied"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_UNKNOWN_STATUS))
	})

	It("returns an error when InitializeStep fails to mark step as in-progress", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkInProgressErr = errors.New("EAGAIN")

		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)
		_, err := hub.InitializeStep("dub-step", mockStream)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("failed to set dub-step to in.progress: EAGAIN"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_UNKNOWN_STATUS))
	})

	It("returns an error when stepwriter MarkComplete fails to mark step as complete", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkCompleteErr = errors.New("ENOENT")

		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)
		step, err := hub.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		err = step.MarkComplete()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("ENOENT"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("returns an error when stepwriter MarkFailed fails to mark step as failed", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkFailedErr = errors.New("EPERM")

		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)
		step, err := hub.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		err = step.MarkFailed()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("EPERM"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("streams status updates from step transitions", func() {
		hubConfig := &services.HubConfig{
			CliToHubPort: cliToHubPort,
		}
		mockChecklistManager := testutils.NewMockChecklistManager()
		hub := services.NewHub(source, target, mockDialer, hubConfig, mockChecklistManager)

		step, err := hub.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		step.MarkComplete()
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_COMPLETE))

		step.MarkFailed()
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_FAILED))
	})
})
