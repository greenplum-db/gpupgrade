package integrations_test

import (
	"os/exec"
	"regexp"
	"strconv"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("gpupgrade_hub", func() {

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
		// XXX for now, assume we're running the utility from PATH
		cmd := exec.Command("gpupgrade_hub")
		err := make(chan error, 1)

		go func() {
			// We expect this to never return.
			err <- cmd.Run()
		}()

		Consistently(err).ShouldNot(Receive())
	})

	It("daemonizes and prints the PID when passed the --daemonize option", func() {
		// XXX for now, assume we're running the utility from PATH
		stdout := gbytes.NewBuffer()
		cmd := exec.Command("gpupgrade_hub", "--daemonize")
		session, err := Start(cmd, stdout, GinkgoWriter)

		Expect(err).NotTo(HaveOccurred())
		Eventually(session).Should(Exit(0))

		// Get the returned PID.
		output := string(stdout.Contents())
		pidmatcher := regexp.MustCompile(`pid (\d+)`)
		matches := pidmatcher.FindStringSubmatch(output)
		Expect(len(matches)).To(Equal(2), `hub output does not contain a PID: "%s"`, output)

		pid, err := strconv.Atoi(pidmatcher.FindStringSubmatch(output)[1])
		Expect(err).NotTo(HaveOccurred())
		Expect(pid).To(BeNumerically(">", 0))

		// Make a best-effort check for process existence...
		// XXX Note that we don't actually verify that this is the hub. Is there
		// a way to do that with standard Go?
		err = syscall.Kill(pid, syscall.Signal(0))
		Expect(err).NotTo(HaveOccurred())
	})
})
