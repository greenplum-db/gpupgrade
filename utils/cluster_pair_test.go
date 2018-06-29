package utils_test

import (
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterPair", func() {
	var (
		filesLaidDown []string
		clusterPair   *utils.ClusterPair
		testExecutor  *testhelper.TestExecutor
		testStateDir  string
		err           error
	)

	BeforeEach(func() {
		testStateDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		testhelper.SetupTestLogger()
		testExecutor = &testhelper.TestExecutor{}
		clusterPair = testutils.CreateSampleClusterPair()
		clusterPair.OldBinDir = "old/path"
		clusterPair.NewBinDir = "new/path"
		clusterPair.OldCluster.Executor = testExecutor
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		filesLaidDown = []string{}
	})

	Describe("WriteClusterConfig", func() {
		It("successfully write cluster config to disk if no file exists", func() {
			sampleCluster := testutils.CreateSampleCluster(-1, 25437, "hostone", "/old/datadir")
			configFilePath := utils.GetConfigFilePath(testStateDir)
			err := utils.WriteClusterConfig(configFilePath, sampleCluster, "/old/bin/dir")
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Open(configFilePath)
			Expect(err).ToNot(HaveOccurred())
		})

		It("successfully write cluster config to disk if file already exists and truncates the rest of the data", func() {
			sampleCluster := testutils.CreateSampleCluster(-1, 25437, "hostone", "/old/datadir")
			configFilePath := utils.GetConfigFilePath(testStateDir)

			f, err := os.OpenFile(configFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			Expect(err).ToNot(HaveOccurred())

			trash_data := `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque auctor luctus ultricies. Curabitur vel tincidunt odio, quis feugiat velit. Mauris cursus purus at felis fringilla, non egestas ante fringilla. In tempus, lectus sed dignissim ultricies, magna est commodo lacus, vel dignissim nunc massa et mauris. Integer quis dolor hendrerit, hendrerit magna at, auctor risus. Vestibulum enim elit, convallis eget est id, feugiat interdum nibh. Interdum et malesuada fames ac ante ipsum primis in faucibus. Aenean efficitur auctor aliquam. Suspendisse potenti.

Curabitur nibh nunc, molestie vitae lectus nec, fermentum consequat est. Etiam interdum quis mi nec volutpat. Curabitur hendrerit convallis ipsum in scelerisque. Suspendisse pharetra mattis auctor. Ut egestas risus enim, a tempus eros ultricies quis. Cras varius mollis aliquet. Phasellus eget tincidunt leo. Sed ut neque turpis. Morbi elementum, tellus quis facilisis consectetur, elit ipsum convallis neque, at elementum purus lacus sodales mauris.

Duis volutpat libero sit amet hendrerit rhoncus. Praesent euismod facilisis elit a tincidunt. Sed porttitor ultrices libero vel imperdiet. Etiam auctor lacinia vehicula. Maecenas ornare, ligula nec consequat vulputate, ex elit lobortis arcu, ut faucibus risus orci vehicula magna. Sed eu porta massa. Praesent fringilla enim id libero suscipit, vitae molestie erat bibendum. Vivamus eu augue in.`
			_, err = f.Write([]byte(trash_data))

			err = utils.WriteClusterConfig(configFilePath, sampleCluster, "/old/bin/dir")
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Open(configFilePath)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = utils.ReadClusterConfig(configFilePath)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
