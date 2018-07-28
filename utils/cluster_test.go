package utils_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/structmatcher"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster", func() {
	var (
		expectedCluster *utils.Cluster
		testStateDir    string
		err             error
	)

	BeforeEach(func() {
		testStateDir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		testhelper.SetupTestLogger()
		expectedCluster = &utils.Cluster{
			Cluster:    testutils.CreateMultinodeSampleCluster("/tmp"),
			BinDir:     "/fake/path",
			ConfigPath: path.Join(testStateDir, "cluster_config.json"),
		}
	})

	AfterEach(func() {
		os.RemoveAll(testStateDir)
	})

	Describe("Commit and Load", func() {
		It("can save a config and successfully load it back in", func() {
			err := expectedCluster.Commit()
			Expect(err).ToNot(HaveOccurred())
			givenCluster := &utils.Cluster{
				ConfigPath: path.Join(testStateDir, "cluster_config.json"),
			}
			err = givenCluster.Load()
			Expect(err).ToNot(HaveOccurred())
			structmatcher.ExpectStructsToMatchExcluding(expectedCluster.Cluster, givenCluster.Cluster, "Executor")
		})
	})
})
