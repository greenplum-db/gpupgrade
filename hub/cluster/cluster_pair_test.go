package cluster_test

import (
	"errors"
	"fmt"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/cluster"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterPair", func() {
	var (
		dir string

		filesLaidDown []string
		commandExecer *testutils.FakeCommandExecer
		errChan       chan error
		outChan       chan []byte
		subject       *cluster.Pair
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger()
		commandExecer = &testutils.FakeCommandExecer{}
		errChan = make(chan error, 2)
		outChan = make(chan []byte, 2)
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})
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
			subject = cluster.NewClusterPair(dir, commandExecer.Exec)

			subject.OldMasterPort = 25437
			subject.NewMasterPort = 35437
			subject.OldMasterDataDirectory = "/old/datadir"
			subject.NewMasterDataDirectory = "/new/datadir"
		})

		It("Logs successfully when things work", func() {
			outChan <- []byte("some output")

			Expect(subject.EitherPostmasterRunning()).To(BeTrue())

			subject.StopEverything("path/to/gpstop")

			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.old/completed"))
			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.new/completed"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/running"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.new/running"))

			Expect(commandExecer.Calls()).To(ContainElement(fmt.Sprintf("bash -c source %[1]s/../greenplum_path.sh; %[1]s/gpstop -a -d %[2]s", "/old/tmp", "/old/datadir")))
			Expect(commandExecer.Calls()).To(ContainElement(fmt.Sprintf("bash -c source %[1]s/../greenplum_path.sh; %[1]s/gpstop -a -d %[2]s", "/new/tmp", "/new/datadir")))
		})

		It("puts failures in the log if there are filesystem errors", func() {
			utils.System.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
				return nil, errors.New("filesystem blowup")
			}

			subject.StopEverything("path/to/gpstop")

			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/in.progress"))
		})

		It("puts Stop failures in the log and leaves files to mark the error", func() {

			Expect(subject.EitherPostmasterRunning()).To(BeTrue())

			errChan <- errors.New("failed")
			subject.StopEverything("path/to/gpstop")

			Expect(filesLaidDown).To(ContainElement("path/to/gpstop/gpstop.old/failed"))
			Expect(filesLaidDown).ToNot(ContainElement("path/to/gpstop/gpstop.old/in.progress"))
		})
	})

	Describe("PostmastersRunning", func() {
		BeforeEach(func() {
			utils.System.ReadFile = func(filename string) ([]byte, error) {
				return []byte(testutils.MASTER_ONLY_JSON), nil
			}

			subject = cluster.NewClusterPair(dir, commandExecer.Exec)
			subject.OldMasterPort = 25437
			subject.NewMasterPort = 35437
			subject.OldMasterDataDirectory = "/old/datadir"
			subject.NewMasterDataDirectory = "/new/datadir"
		})

		It("returns true if both postmaster processes are running", func() {
			Expect(subject.EitherPostmasterRunning()).To(BeTrue())
		})

		It("returns true if only old postmaster is running", func() {
			errChan <- nil
			errChan <- errors.New("failed")
			Expect(subject.EitherPostmasterRunning()).To(BeTrue())
		})

		It("returns true if only new postmaster is running", func() {
			errChan <- errors.New("failed")
			errChan <- nil
			Expect(subject.EitherPostmasterRunning()).To(BeTrue())
		})

		It("returns false if both postmaster processes are down", func() {
			errChan <- errors.New("failed")
			errChan <- errors.New("failed")
			Expect(subject.EitherPostmasterRunning()).To(BeFalse())
		})
	})
	Describe("GetMasterPorts", func() {
		BeforeEach(func() {
			numInvocations := 0
			utils.System.ReadFile = func(filename string) ([]byte, error) {
				if (numInvocations == 0) {
					numInvocations += 1
					return []byte(testutils.MASTER_ONLY_JSON), nil
				}
				return []byte(testutils.NEW_MASTER_JSON), nil
			}

			subject = cluster.NewClusterPair(dir, commandExecer.Exec)
		})

		It("returns both master ports correctly", func() {
			oldMasterPort, newMasterPort, err := subject.GetMasterPorts()
			Expect(err).To(BeNil())
			Expect(oldMasterPort).To(Equal(25437))
			Expect(newMasterPort).To(Equal(35437))
		})

	})
})
