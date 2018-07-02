package services_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent/services"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CommandListener", func() {
	var (
		agent        *services.AgentServer
		testExecutor *testhelper.TestExecutor
		dir          string
	)

	BeforeEach(func() {
		testhelper.SetupTestLogger()

		testExecutor = &testhelper.TestExecutor{}

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		agentConfig := services.AgentConfig{StateDir: dir}
		agent = services.NewAgentServer(testExecutor, agentConfig)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("returns a status string for each DBID passed from the hub", func() {
		status, err := agent.CheckConversionStatus(nil, &pb.CheckConversionStatusRequest{
			Segments: []*pb.SegmentInfo{{
				Content: 1,
				Dbid:    3,
				DataDir: "/old/data/dir",
			}, {
				Content: -1,
				Dbid:    1,
				DataDir: "/old/dir",
			}},
			Hostname: "localhost",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(status.GetStatuses()).To(Equal([]string{
			"PENDING - DBID 1 - CONTENT ID -1 - MASTER - localhost",
			"PENDING - DBID 3 - CONTENT ID 1 - PRIMARY - localhost",
		}))
	})

	It("returns running for segments that have the upgrade in progress", func() {
		err := os.MkdirAll(filepath.Join(dir, "pg_upgrade", "seg-1"), 0700)
		Expect(err).ToNot(HaveOccurred())
		fd, err := os.Create(filepath.Join(dir, "pg_upgrade", "seg-1", ".inprogress"))
		Expect(err).ToNot(HaveOccurred())
		fd.Close()

		testExecutor.LocalOutput = "pid1"

		status, err := agent.CheckConversionStatus(nil, &pb.CheckConversionStatusRequest{
			Segments: []*pb.SegmentInfo{{
				Content: 1,
				Dbid:    3,
				DataDir: "/old/data/dir",
			}, {
				Content: -1,
				Dbid:    1,
				DataDir: "/old/dir",
			}},
			Hostname: "localhost",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(status.GetStatuses()).To(Equal([]string{
			"PENDING - DBID 1 - CONTENT ID -1 - MASTER - localhost",
			"RUNNING - DBID 3 - CONTENT ID 1 - PRIMARY - localhost",
		}))
	})

	It("returns COMPLETE for segments that have completed the upgrade", func() {
		err := os.MkdirAll(filepath.Join(dir, "pg_upgrade", "seg--1"), 0700)
		Expect(err).ToNot(HaveOccurred())
		fd, err := os.Create(filepath.Join(dir, "pg_upgrade", "seg--1", ".done"))
		Expect(err).ToNot(HaveOccurred())
		fd.WriteString("Upgrade complete\n")
		fd.Close()

		status, err := agent.CheckConversionStatus(nil, &pb.CheckConversionStatusRequest{
			Segments: []*pb.SegmentInfo{{
				Content: 1,
				Dbid:    3,
				DataDir: "/old/data/dir",
			}, {
				Content: -1,
				Dbid:    1,
				DataDir: "/old/dir",
			}},
			Hostname: "localhost",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(status.GetStatuses()).To(Equal([]string{
			"COMPLETE - DBID 1 - CONTENT ID -1 - MASTER - localhost",
			"PENDING - DBID 3 - CONTENT ID 1 - PRIMARY - localhost",
		}))
	})

	It("returns an error if no segments are passed", func() {
		request := &pb.CheckConversionStatusRequest{
			Segments: []*pb.SegmentInfo{},
			Hostname: "localhost",
		}

		_, err := agent.CheckConversionStatus(nil, request)
		Expect(err).To(HaveOccurred())
	})
})
