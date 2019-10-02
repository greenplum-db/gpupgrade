package commanders

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	. "github.com/onsi/gomega"
)

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func HowManyHubsRunning_0_Main() {
	fmt.Print("0")
}
func HowManyHubsRunning_1_Main() {
	fmt.Print("1")
}
func HowManyHubsRunning_badoutput_Main() {
	fmt.Print("bengie")
}

func GpupgradeHub_good_Main() {
	fmt.Print("Hi, Hub started.")
}

func GpupgradeHub_bad_Main() {
	fmt.Fprint(os.Stderr, "Sorry, Hub could not be started.")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		HowManyHubsRunning_0_Main,
		HowManyHubsRunning_1_Main,
		HowManyHubsRunning_badoutput_Main,
		GpupgradeHub_good_Main,
		GpupgradeHub_bad_Main,
	)
}

var (
	ctrl *gomock.Controller
	g    *GomegaWithT
)

func setup(t *testing.T) {
	ctrl = gomock.NewController(t)
	g = NewGomegaWithT(t)
	execCommandHubStart = nil
	execCommandHubCount = nil
}

func teardown() {
	ctrl.Finish()
	execCommandHubStart = exec.Command
	execCommandHubCount = exec.Command
}

func TestNoHubIsAlreadyRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_0_Main)
	numHubs, err := HowManyHubsRunning()
	g.Expect(err).To(BeNil())
	g.Expect(numHubs).To(Equal(0))
}

func TestHubIsAlreadyRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_1_Main)
	numHubs, err := HowManyHubsRunning()
	g.Expect(err).To(BeNil())
	g.Expect(numHubs).To(Equal(1))
}

func TestHowManyHubsRunningFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_badoutput_Main)
	_, err := HowManyHubsRunning()
	g.Expect(err).ToNot(BeNil())
}

func TestWeCanStartHub(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_0_Main)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main)
	err := StartHub()
	g.Expect(err).To(BeNil())
}

func TestStartHubHFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_badoutput_Main)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main)
	err := StartHub()
	g.Expect(err).ToNot(BeNil())
}

func TestStartHubRestartFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_1_Main)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main)
	err := StartHub()
	g.Expect(err).ToNot(BeNil())
}

func TestStartHubBadExec(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(HowManyHubsRunning_0_Main)
	execCommandHubStart = exectest.NewCommand(GpupgradeHub_bad_Main)
	err := StartHub()
	g.Expect(err).ToNot(BeNil())
}

func TestInitialize(t *testing.T) {
	setup(t)
	defer teardown()

	clientStream := mock_idl.NewMockCliToHub_InitializeClient(ctrl)
	clientStream.EXPECT().Recv().Return(nil, io.EOF)

	client := mock_idl.NewMockCliToHubClient(ctrl)
	client.EXPECT().Initialize(
		gomock.Any(),
		&idl.InitializeRequest{OldBinDir: "olddir", NewBinDir: "newdir", OldPort: 22},
	).Return(clientStream, nil)

	client.EXPECT().StatusUpgrade(
		gomock.Any(),
		&idl.StatusUpgradeRequest{},
	).Return(&idl.StatusUpgradeReply{}, nil).AnyTimes()

	err := Initialize(client, "olddir", "newdir", 22)
	g.Expect(err).To(BeNil())
}

func TestCannotInitialize(t *testing.T) {
	setup(t)
	defer teardown()

	client := mock_idl.NewMockCliToHubClient(ctrl)
	client.EXPECT().Initialize(
		gomock.Any(),
		&idl.InitializeRequest{OldBinDir: "olddir", NewBinDir: "newdir", OldPort: 22},
	).Return(nil, errors.New("something failed with gRPC"))

	client.EXPECT().StatusUpgrade(
		gomock.Any(),
		&idl.StatusUpgradeRequest{},
	).Return(&idl.StatusUpgradeReply{}, nil).AnyTimes()

	err := Initialize(client, "olddir", "newdir", 22)
	g.Expect(err).ToNot(BeNil())
}

func TestDisplayingTheStepStatuses(t *testing.T) {
	g := NewGomegaWithT(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_idl.NewMockCliToHubClient(ctrl)
	clientStream := mock_idl.NewMockCliToHub_InitializeClient(ctrl)

	client.EXPECT().Initialize(
		gomock.Any(),
		&idl.InitializeRequest{
			OldBinDir: "old",
			NewBinDir: "new",
			OldPort: 12345,
		},
	).Return(clientStream, nil)
	gomock.InOrder(
		clientStream.EXPECT().Recv().Return(&idl.UpgradeStream{
			Type: idl.UpgradeStream_STEP_STATUS,
			Status: &idl.UpgradeStepStatus{
				Step:                 idl.UpgradeSteps_CONFIG,
				Status:               idl.StepStatus_PENDING,
			},
		}, nil),
		clientStream.EXPECT().Recv().Return(&idl.UpgradeStream{
			Type: idl.UpgradeStream_STEP_STATUS,
			Status: &idl.UpgradeStepStatus{
				Step:                 idl.UpgradeSteps_CONFIG,
				Status:               idl.StepStatus_COMPLETE,
			},
		}, nil),
		clientStream.EXPECT().Recv().Return(&idl.UpgradeStream{
			Type: idl.UpgradeStream_STEP_STATUS,
			Status: &idl.UpgradeStepStatus{
				Step:                 idl.UpgradeSteps_START_AGENTS,
				Status:               idl.StepStatus_RUNNING,
			},
		}, nil),
		clientStream.EXPECT().Recv().Return(nil, io.EOF),
	)

	rOut, wOut, _ := os.Pipe()
	tmpOut := os.Stdout
	defer func() {
		os.Stdout = tmpOut
	}()
	os.Stdout = wOut
	go func() {
		defer wOut.Close()
		err := Initialize(client, "old","new", 12345)
		g.Expect(err).To(BeNil())
	}()
	actualOut, _ := ioutil.ReadAll(rOut)

	g.Expect(string(actualOut)).To(MatchRegexp(`retrieving configs[.]{3}\s*\[COMPLETE\]\s*\nstarting agents[.]{3}\s*\[IN_PROGRESS\]\s*`))
}
