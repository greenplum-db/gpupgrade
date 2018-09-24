package commanders_test

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	pb "github.com/greenplum-db/gpupgrade/idl"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var (
	stdoutRead  *os.File
	stdoutWrite *os.File
	stdoutSaved *os.File
)

func getStdoutContents() string {
	stdoutWrite.Close()
	contents, _ := ioutil.ReadAll(stdoutRead)
	return string(contents)
}

var _ = Describe("Reporter", func() {
	var (
		spyClient   *spyCliToHubClient
		testLogFile *gbytes.Buffer
		reporter    *commanders.Reporter
		ctrl        *gomock.Controller
	)

	BeforeEach(func() {
		spyClient = newSpyCliToHubClient()
		_, _, testLogFile = testhelper.SetupTestLogger()
		reporter = commanders.NewReporter(spyClient)
		ctrl = gomock.NewController(GinkgoT())

		stdoutRead, stdoutWrite, _ = os.Pipe()
		stdoutSaved = os.Stdout
		os.Stdout = stdoutWrite
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		ctrl.Finish()
		os.Stdout = stdoutSaved
	})

	Describe("StatusConversion", func() {
		It("prints cluster status returned from hub", func() {
			status := []string{"cluster status"}
			spyClient.statusConversionReply = &pb.StatusConversionReply{
				ConversionStatuses: status,
			}

			err := reporter.OverallConversionStatus()
			Expect(err).ToNot(HaveOccurred())

			Expect(spyClient.statusConversionCount).To(Equal(1))
			Expect(getStdoutContents()).To(ContainSubstring("cluster status"))
		})

		It("returns an error upon a failure", func() {
			spyClient.err = errors.New("error error")
			err := reporter.OverallConversionStatus()
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the hub returns no error, but the reply is empty", func() {
			By("having an empty conversion status")
			spyClient.statusConversionReply = &pb.StatusConversionReply{}
			err := reporter.OverallConversionStatus()
			Expect(err).To(HaveOccurred())

			Expect(spyClient.statusConversionCount).To(Equal(1))
		})
	})

	Describe("StatusUpgrade", func() {
		It("returns an error upon a failure", func() {
			spyClient.err = errors.New("some error")
			err := reporter.OverallUpgradeStatus()
			Expect(err).To(HaveOccurred())
		})

		It("sends all the right messages to the logger in the right order when reply contains multiple step-statuses", func() {
			spyClient.statusUpgradeReply = &pb.StatusUpgradeReply{
				ListOfUpgradeStepStatuses: []*pb.UpgradeStepStatus{
					{Step: pb.UpgradeSteps_INIT_CLUSTER, Status: pb.StepStatus_RUNNING},
					{Step: pb.UpgradeSteps_CONVERT_MASTER, Status: pb.StepStatus_PENDING},
				},
			}
			err := reporter.OverallUpgradeStatus()
			Expect(err).ToNot(HaveOccurred())
			contents := getStdoutContents()
			Expect(contents).To(ContainSubstring("RUNNING - Initialize new cluster"))
			Expect(contents).To(ContainSubstring("PENDING - Run pg_upgrade on master"))
		})

		It("returns an error when the hub returns no error, but the reply has an empty list", func() {
			By("having an empty status list")
			spyClient.statusUpgradeReply = &pb.StatusUpgradeReply{}
			err := reporter.OverallUpgradeStatus()
			Expect(err).To(HaveOccurred())

			Expect(spyClient.statusUpgradeCount).To(Equal(1))
		})

		DescribeTable("UpgradeStep Messages, basic cases where hub might return only one status",
			func(step pb.UpgradeSteps, status pb.StepStatus, expected string) {
				spyClient.statusUpgradeReply = &pb.StatusUpgradeReply{
					ListOfUpgradeStepStatuses: []*pb.UpgradeStepStatus{
						{Step: step, Status: status},
					},
				}
				err := reporter.OverallUpgradeStatus()
				Expect(err).ToNot(HaveOccurred())
				Expect(getStdoutContents()).To(ContainSubstring(expected))
			},
			Entry("unknown step", pb.UpgradeSteps_UNKNOWN_STEP, pb.StepStatus_PENDING, "PENDING - Unknown step"),
			Entry("configuration check", pb.UpgradeSteps_CONFIG, pb.StepStatus_RUNNING, "RUNNING - Configuration Check"),
			Entry("install binaries on segments", pb.UpgradeSteps_SEGINSTALL, pb.StepStatus_COMPLETE, "COMPLETE - Install binaries on segments"),
			Entry("prepare init cluster", pb.UpgradeSteps_INIT_CLUSTER, pb.StepStatus_FAILED, "FAILED - Initialize new cluster"),
			Entry("upgrade on master", pb.UpgradeSteps_CONVERT_MASTER, pb.StepStatus_PENDING, "PENDING - Run pg_upgrade on master"),
			Entry("shutdown cluster", pb.UpgradeSteps_SHUTDOWN_CLUSTERS, pb.StepStatus_PENDING, "PENDING - Shutdown clusters"),
			Entry("reconfigure ports", pb.UpgradeSteps_RECONFIGURE_PORTS, pb.StepStatus_PENDING, "PENDING - Adjust upgraded cluster ports"),
		)
	})
})
