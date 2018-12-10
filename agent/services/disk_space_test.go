package services_test

import (
	"github.com/greenplum-db/gpupgrade/agent/services"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CommandListener", func() {
	var (
		testLogFile *gbytes.Buffer
	)

	BeforeEach(func() {
		_, _, testLogFile = testhelper.SetupTestLogger()
	})

	AfterEach(func() {
		//any mocking of utils.System function pointers should be reset by calling InitializeSystemFunctions
		utils.System = utils.InitializeSystemFunctions()
	})

	It("returns information that a helper function got about filesystems", func() {
		getDiskUsage := func() (map[string]float64, error) {
			fakeDiskUsage := make(map[string]float64)
			fakeDiskUsage["/data"] = 25.4
			return fakeDiskUsage, nil
		}
		listener := &services.AgentServer{GetDiskUsage: getDiskUsage}

		resp, err := listener.CheckDiskSpaceOnAgents(nil, &idl.CheckDiskSpaceRequestToAgent{})
		Expect(err).To(BeNil())
		for _, val := range resp.ListOfFileSysUsage {
			if val.Filesystem == "/data" {
				Expect(val.Usage).To(BeNumerically("~", 25.4, 0.001))
				break

			}
		}
	})

	It("returns an error if the helper function fails", func() {
		getDiskUsage := func() (map[string]float64, error) {
			return nil, errors.New("fake error")
		}
		listener := &services.AgentServer{GetDiskUsage: getDiskUsage}
		_, err := listener.CheckDiskSpaceOnAgents(nil, &idl.CheckDiskSpaceRequestToAgent{})
		Expect(err).To(HaveOccurred())
		Expect(string(testLogFile.Contents())).To(ContainSubstring("fake error"))
	})
})
