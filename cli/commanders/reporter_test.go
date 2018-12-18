package commanders_test

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
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
		spyClient *spyCliToHubClient
		reporter  *commanders.Reporter
		ctrl      *gomock.Controller
	)

	BeforeEach(func() {
		spyClient = newSpyCliToHubClient()
		testhelper.SetupTestLogger()
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
			status := []*idl.PrimaryStatus{{Status: idl.StepStatus_PENDING}}
			spyClient.statusConversionReply = &idl.StatusConversionReply{
				ConversionStatuses: status,
			}

			err := reporter.OverallConversionStatus()
			Expect(err).ToNot(HaveOccurred())

			Expect(spyClient.statusConversionCount).To(Equal(1))
			Expect(getStdoutContents()).To(ContainSubstring("PENDING"))
		})

		It("returns an error upon a failure", func() {
			spyClient.err = errors.New("error error")
			err := reporter.OverallConversionStatus()
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the hub returns no error, but the reply is empty", func() {
			By("having an empty conversion status")
			spyClient.statusConversionReply = &idl.StatusConversionReply{}
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
			spyClient.statusUpgradeReply = &idl.StatusUpgradeReply{
				ListOfUpgradeStepStatuses: []*idl.UpgradeStepStatus{
					{Step: idl.UpgradeSteps_INIT_CLUSTER, Status: idl.StepStatus_RUNNING},
					{Step: idl.UpgradeSteps_CONVERT_MASTER, Status: idl.StepStatus_PENDING},
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
			spyClient.statusUpgradeReply = &idl.StatusUpgradeReply{}
			err := reporter.OverallUpgradeStatus()
			Expect(err).To(HaveOccurred())

			Expect(spyClient.statusUpgradeCount).To(Equal(1))
		})

		DescribeTable("UpgradeStep Messages, basic cases where hub might return only one status",
			func(step idl.UpgradeSteps, status idl.StepStatus, expected string) {
				spyClient.statusUpgradeReply = &idl.StatusUpgradeReply{
					ListOfUpgradeStepStatuses: []*idl.UpgradeStepStatus{
						{Step: step, Status: status},
					},
				}
				err := reporter.OverallUpgradeStatus()
				Expect(err).ToNot(HaveOccurred())
				Expect(getStdoutContents()).To(ContainSubstring(expected))
			},
			Entry("unknown step", idl.UpgradeSteps_UNKNOWN_STEP, idl.StepStatus_PENDING, "PENDING - Unknown step"),
			Entry("configuration check", idl.UpgradeSteps_CONFIG, idl.StepStatus_RUNNING, "RUNNING - Configuration Check"),
			Entry("install binaries on segments", idl.UpgradeSteps_SEGINSTALL, idl.StepStatus_COMPLETE, "COMPLETE - Install binaries on segments"),
			Entry("prepare init cluster", idl.UpgradeSteps_INIT_CLUSTER, idl.StepStatus_FAILED, "FAILED - Initialize new cluster"),
			Entry("upgrade on master", idl.UpgradeSteps_CONVERT_MASTER, idl.StepStatus_PENDING, "PENDING - Run pg_upgrade on master"),
			Entry("shutdown cluster", idl.UpgradeSteps_SHUTDOWN_CLUSTERS, idl.StepStatus_PENDING, "PENDING - Shutdown clusters"),
			Entry("reconfigure ports", idl.UpgradeSteps_RECONFIGURE_PORTS, idl.StepStatus_PENDING, "PENDING - Adjust upgraded cluster ports"),
		)
	})
})
