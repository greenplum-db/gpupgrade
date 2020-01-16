package hub_test

import (
	"context"
	"net"
	"os"
	"reflect"
	"strconv"
	"testing"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/greenplum-db/gp-common-go-libs/cluster"

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
	LastStatus idl.StepStatus
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
		source         *utils.Cluster
		target         *utils.Cluster
		conf           *hub.Config
		err            error
		mockDialer     hub.Dialer
		mockStream     *msgStream
		useLinkMode    bool
	)

	BeforeEach(func() {
		agentA, mockDialer, hubToAgentPort = mock_agent.NewMockAgentServer()
		source, target = testutils.CreateMultinodeSampleClusterPair("/tmp")
		useLinkMode = false
		conf = &hub.Config{source, target, cliToHubPort, hubToAgentPort, useLinkMode}
		mockStream = &msgStream{}
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		agentA.Stop()
	})

	It("will return from Start() with an error if Stop() is called first", func() {
		h := hub.New(conf, mockDialer, "", nil)

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

		h := hub.New(conf, mockDialer, "", nil)

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
		h := hub.New(conf, mockDialer, "", nil)
		done := make(chan bool, 1)

		go func() {
			h.Start()
			done <- true
		}()
		h.Stop(true)

		Eventually(done).Should(Receive())
	})

	It("closes open connections when shutting down", func() {
		h := hub.New(conf, mockDialer, "", nil)
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
		h := hub.New(conf, mockDialer, "", nil)

		conns, err := h.AgentConns()
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
		h := hub.New(conf, mockDialer, "", nil)

		newConns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		savedConns, err := h.AgentConns()
		Expect(err).ToNot(HaveOccurred())

		Expect(newConns).To(ConsistOf(savedConns))
	})

	// XXX This test takes 1.5 seconds because of EnsureConnsAreReady(...)
	It("returns an error if any connections have non-ready states", func() {
		h := hub.New(conf, mockDialer, "", nil)

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

		h := hub.New(conf, errDialer, "", nil)

		_, err := h.AgentConns()
		Expect(err).To(HaveOccurred())
	})

	It("successfully initializes step by marking it as in-progress with status running", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		h := hub.New(conf, mockDialer, "", mockChecklistManager)
		h.InitializeStep("dub-step", mockStream)

		Expect(mockChecklistManager.GetStepReader("dub-step").Status()).To(Equal(idl.StepStatus_RUNNING))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("returns an error when InitializeStep fails to reset state directory", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.ResetStateDirErr = errors.New("permission denied")

		h := hub.New(conf, mockDialer, "", mockChecklistManager)
		_, err := h.InitializeStep("dub-step", mockStream)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("failed to reset state directory: permission denied"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_UNKNOWN_STATUS))
	})

	It("returns an error when InitializeStep fails to mark step as in-progress", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkInProgressErr = errors.New("EAGAIN")

		h := hub.New(conf, mockDialer, "", mockChecklistManager)
		_, err := h.InitializeStep("dub-step", mockStream)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("failed to set dub-step to in.progress: EAGAIN"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_UNKNOWN_STATUS))
	})

	It("returns an error when stepwriter MarkComplete fails to mark step as complete", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkCompleteErr = errors.New("ENOENT")

		h := hub.New(conf, mockDialer, "", mockChecklistManager)
		step, err := h.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		err = step.MarkComplete()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("ENOENT"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("returns an error when stepwriter MarkFailed fails to mark step as failed", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		mockChecklistManager.StepWriter.MarkFailedErr = errors.New("EPERM")

		h := hub.New(conf, mockDialer, "", mockChecklistManager)
		step, err := h.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		err = step.MarkFailed()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("EPERM"))
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_RUNNING))
	})

	It("streams status updates from step transitions", func() {
		mockChecklistManager := testutils.NewMockChecklistManager()
		h := hub.New(conf, mockDialer, "", mockChecklistManager)

		step, err := h.InitializeStep("dub-step", mockStream)
		Expect(err).ToNot(HaveOccurred())

		step.MarkComplete()
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_COMPLETE))

		step.MarkFailed()
		Expect(mockStream.LastStatus).To(Equal(idl.StepStatus_FAILED))
	})
})

func TestHubSaveConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	source.Executor = new(cluster.GPDBExecutor)
	target.Executor = new(cluster.GPDBExecutor)
	useLinkMode := false
	conf := &hub.Config{source, target, 12345, 54321, useLinkMode}

	h := hub.New(conf, nil, "", nil)

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
