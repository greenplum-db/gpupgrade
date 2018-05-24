package services_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/greenplum-db/gpupgrade/hub/services"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/greenplum-db/gpupgrade/utils"
)

var _ = Describe("UpgradeShareOids", func() {
	var (
		reader        *testutils.SpyReader
		hub           *services.Hub
		dir           string
		commandExecer *testutils.FakeCommandExecer
		errChan       chan error
		outChan       chan []byte
		stubRemoteExecutor *testutils.StubRemoteExecutor
	)

	BeforeEach(func() {
		reader = &testutils.SpyReader{
			Hostnames: []string{"hostone", "hosttwo"},
		}

		var err error
		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		errChan = make(chan error, 2)
		outChan = make(chan []byte, 2)
		commandExecer = &testutils.FakeCommandExecer{}
		commandExecer.SetOutput(&testutils.FakeCommand{
			Err: errChan,
			Out: outChan,
		})
		stubRemoteExecutor = testutils.NewStubRemoteExecutor()
		hub = services.NewHub(nil, reader, grpc.DialContext, commandExecer.Exec, &services.HubConfig{
			StateDir: dir,
		}, stubRemoteExecutor)
	})

	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
		os.RemoveAll(dir)
	})

	It("copies files to each host", func() {
		_, err := hub.UpgradeShareOids(nil, &pb.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		hostnames, err := reader.GetHostnames()
		Expect(err).ToNot(HaveOccurred())

		Eventually(commandExecer.GetNumInvocations).Should(Equal(len(hostnames)))

		Expect(commandExecer.Calls()).To(Equal([]string{
			fmt.Sprintf("bash -c rsync -rzpogt %s/pg_upgrade/pg_upgrade_dump_*_oids.sql gpadmin@hostone:%s/pg_upgrade", dir, dir),
			fmt.Sprintf("bash -c rsync -rzpogt %s/pg_upgrade/pg_upgrade_dump_*_oids.sql gpadmin@hosttwo:%s/pg_upgrade", dir, dir),
		}))
	})

	It("copies all files even if rsync fails for a host", func() {
		errChan <- errors.New("failure")

		_, err := hub.UpgradeShareOids(nil, &pb.UpgradeShareOidsRequest{})
		Expect(err).ToNot(HaveOccurred())

		hostnames, err := reader.GetHostnames()
		Expect(err).ToNot(HaveOccurred())

		Eventually(commandExecer.GetNumInvocations).Should(Equal(len(hostnames)))
	})
})
