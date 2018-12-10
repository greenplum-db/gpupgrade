package services_test

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeReconfigurePorts", func() {
	var (
		testExecutor *testhelper.TestExecutor
	)

	BeforeEach(func() {
		numInvocations := 0
		utils.System.ReadFile = func(filename string) ([]byte, error) {
			if numInvocations == 0 {
				numInvocations++
				return []byte(testutils.MASTER_ONLY_JSON), nil
			} else {
				return []byte(testutils.NEW_MASTER_JSON), nil
			}
		}

		testExecutor = &testhelper.TestExecutor{}
		source.Executor = testExecutor
		targetMaster := target.Segments[-1]
		targetMaster.Port = 17432
		target.Segments[-1] = targetMaster
	})

	It("reconfigures port in postgresql.conf on master", func() {
		reply, err := hub.UpgradeReconfigurePorts(nil, &idl.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(Equal(&idl.UpgradeReconfigurePortsReply{}))
		Expect(err).To(BeNil())
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 17432, 15432, filepath.Join(dir, "seg-1"))))
	})

	It("returns err if reconfigure cmd fails", func() {
		testExecutor.LocalError = errors.New("boom")
		reply, err := hub.UpgradeReconfigurePorts(nil, &idl.UpgradeReconfigurePortsRequest{})
		Expect(reply).To(BeNil())
		Expect(err).ToNot(BeNil())
		Expect(testExecutor.LocalCommands[0]).To(ContainSubstring(fmt.Sprintf(services.SedAndMvString, 17432, 15432, filepath.Join(dir, "seg-1"))))
	})

})
