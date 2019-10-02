package stream_step_status

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	. "github.com/onsi/gomega"
)

func TestStreamStatus(t *testing.T) {
	g := NewGomegaWithT(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_InitializeServer(ctrl)
	mockStream.EXPECT().
		Send(&idl.UpgradeStream{
			Type: idl.UpgradeStream_STEP_STATUS,
			Status: &idl.UpgradeStepStatus{
				Step:   idl.UpgradeSteps_START_AGENTS,
				Status: idl.StepStatus_RUNNING,
			},
		}).
		Times(1)

	err := StreamStepStatus(mockStream, idl.UpgradeSteps_START_AGENTS, idl.StepStatus_RUNNING)
	g.Expect(err).To(BeNil())
}

func TestStreamStatusWhenFailure(t *testing.T) {
	g := NewGomegaWithT(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_InitializeServer(ctrl)
	expectedError := errors.New("FOMO vs YOLO")
	mockStream.
		EXPECT().
		Send(gomock.Any()).
		Return(expectedError).
		Times(1)

	err := StreamStepStatus(mockStream, idl.UpgradeSteps_START_AGENTS, idl.StepStatus_RUNNING)
	g.Expect(err).To(Equal(expectedError))
}
