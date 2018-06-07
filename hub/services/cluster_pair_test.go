package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterPair", func() {
	var (
		filesLaidDown []string
		clusterPair   *services.ClusterPair
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

	Describe("StopEverything(), shutting down both clusters", func() {
		BeforeEach(func() {
			// fake out system utilities
			numInvocations := 0
			utils.System.ReadFile = func(filename string) ([]byte, error) {
				if numInvocations == 0 {
					numInvocations++
					return []byte(testutils.MASTER_ONLY_JSON), nil
				} else {
					return []byte(testutils.NEW_MASTER_JSON), nil
				}
			}
			utils.System.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
				filesLaidDown = append(filesLaidDown, name)
				return nil, nil
			}
			utils.System.Remove = func(name string) error {
				filteredFiles := make([]string, 0)
				for _, file := range filesLaidDown {
					if file != name {
						filteredFiles = append(filteredFiles, file)
					}
				}
				filesLaidDown = filteredFiles
				return nil
			}
		})

		It("Logs successfully when things work", func() {
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeTrue())
			Expect(newRunning).To(BeTrue())

			clusterPair.StopEverything("path/to/gpstop", oldRunning, newRunning)

			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.old/completed"))
			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.new/completed"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/running"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.new/running"))

			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstop -a -d %s", "old/path", "old/path", "/old/datadir")))
			Expect(testExecutor.LocalCommands).To(ContainElement(fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstop -a -d %s", "new/path", "new/path", "/new/datadir")))
		})

		It("puts failures in the log if there are filesystem errors", func() {
			utils.System.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
				return nil, errors.New("filesystem blowup")
			}

			clusterPair.StopEverything("path/to/gpstop", true, true)

			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/in.progress"))
		})

		It("puts Stop failures in the log and leaves files to mark the error", func() {
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeTrue())
			Expect(newRunning).To(BeTrue())

			testExecutor.LocalError = errors.New("generic error")
			clusterPair.StopEverything("path/to/gpstop", oldRunning, newRunning)

			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.old/failed"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/in.progress"))
		})
	})

	Describe("PostmastersRunning", func() {
		BeforeEach(func() {
			utils.System.ReadFile = func(filename string) ([]byte, error) {
				return []byte(testutils.MASTER_ONLY_JSON), nil
			}
			clusterPair.OldCluster.Executor = &testhelper.TestExecutor{}
		})
		It("returns true, true if both postmaster processes are running", func() {
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeTrue())
			Expect(newRunning).To(BeTrue())
		})
		It("returns true, false if only old postmaster is running", func() {
			clusterPair.OldCluster.Executor = &testhelper.TestExecutor{
				LocalError:     errors.New("failed"),
				ErrorOnExecNum: 2,
			}
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeTrue())
			Expect(newRunning).To(BeFalse())
		})
		It("returns false, true if only new postmaster is running", func() {
			clusterPair.OldCluster.Executor = &testhelper.TestExecutor{
				LocalError:     errors.New("failed"),
				ErrorOnExecNum: 1,
			}
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeFalse())
			Expect(newRunning).To(BeTrue())
		})
		It("returns false, false if both postmaster processes are down", func() {
			clusterPair.OldCluster.Executor = &testhelper.TestExecutor{
				LocalError: errors.New("failed"),
			}
			oldRunning, newRunning := clusterPair.EitherPostmasterRunning()
			Expect(oldRunning).To(BeFalse())
			Expect(newRunning).To(BeFalse())
		})
	})

	Describe("WriteClusterConfig", func() {
		It("successfully write cluster config to disk if no file exists", func() {
			sampleCluster := testutils.CreateSampleCluster(-1, 25437, "hostone", "/old/datadir")
			configFilePath := services.GetConfigFilePath(testStateDir)
			err := services.WriteClusterConfig(configFilePath, sampleCluster, "/old/bin/dir")
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Open(configFilePath)
			Expect(err).ToNot(HaveOccurred())
		})

		It("successfully write cluster config to disk if file already exists and truncates the rest of the data", func() {
			sampleCluster := testutils.CreateSampleCluster(-1, 25437, "hostone", "/old/datadir")
			configFilePath := services.GetConfigFilePath(testStateDir)

			f, err := os.OpenFile(configFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			Expect(err).ToNot(HaveOccurred())

			trash_data := `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque auctor luctus ultricies. Curabitur vel tincidunt odio, quis feugiat velit. Mauris cursus purus at felis fringilla, non egestas ante fringilla. In tempus, lectus sed dignissim ultricies, magna est commodo lacus, vel dignissim nunc massa et mauris. Integer quis dolor hendrerit, hendrerit magna at, auctor risus. Vestibulum enim elit, convallis eget est id, feugiat interdum nibh. Interdum et malesuada fames ac ante ipsum primis in faucibus. Aenean efficitur auctor aliquam. Suspendisse potenti.

Curabitur nibh nunc, molestie vitae lectus nec, fermentum consequat est. Etiam interdum quis mi nec volutpat. Curabitur hendrerit convallis ipsum in scelerisque. Suspendisse pharetra mattis auctor. Ut egestas risus enim, a tempus eros ultricies quis. Cras varius mollis aliquet. Phasellus eget tincidunt leo. Sed ut neque turpis. Morbi elementum, tellus quis facilisis consectetur, elit ipsum convallis neque, at elementum purus lacus sodales mauris.

Duis volutpat libero sit amet hendrerit rhoncus. Praesent euismod facilisis elit a tincidunt. Sed porttitor ultrices libero vel imperdiet. Etiam auctor lacinia vehicula. Maecenas ornare, ligula nec consequat vulputate, ex elit lobortis arcu, ut faucibus risus orci vehicula magna. Sed eu porta massa. Praesent fringilla enim id libero suscipit, vitae molestie erat bibendum. Vivamus eu augue in.`
			_, err = f.Write([]byte(trash_data))

			err = services.WriteClusterConfig(configFilePath, sampleCluster, "/old/bin/dir")
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Open(configFilePath)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = services.ReadClusterConfig(configFilePath)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
