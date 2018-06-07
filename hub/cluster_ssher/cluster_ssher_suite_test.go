package cluster_ssher

import (
	"testing"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Ssher Suite")
}

var _ = BeforeSuite(func() {
	testhelper.SetupTestLogger()
})

var _ = AfterEach(func() {
	utils.System = utils.InitializeSystemFunctions()
})
