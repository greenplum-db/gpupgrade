package integrations_test

import (
	"os/exec"

	"github.com/greenplum-db/gpupgrade/cli/commanders"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("gpupgrade hub", func() {

	// XXX We should be testing the locally built artifacts, and killing only
	// hubs that are started as part of this test. The current logic will break
	// functional installed systems.
	BeforeEach(func() {
		killHub()
	})

	AfterEach(func() {
		killHub()
	})

	It("does not daemonize unless explicitly told to", func() {
		err := commanders.CreateStateDir()
		Expect(err).ToNot(HaveOccurred())
		err = commanders.CreateInitialClusterConfigs()
		Expect(err).ToNot(HaveOccurred())

		cmd := exec.Command("gpupgrade", "hub")
		done := make(chan error, 1)

		go func() {
			// We expect this to never return.
			done <- cmd.Run()
		}()

		Consistently(done).ShouldNot(Receive())
	})
})
