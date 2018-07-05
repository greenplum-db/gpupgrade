package integrations_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Start Hub", func() {

	BeforeEach(func() {
		killHub()
	})

	AfterEach(func() {
		killHub()
	})

	It("finds the right hub binary and starts a daemonized process", func() {
		gpUpgradeSession := runCommand("prepare", "start-hub")
		Eventually(gpUpgradeSession).Should(Exit(0))

		verificationCmd := exec.Command("bash", "-c", `ps -ef | grep -Gq "[g]pupgrade_hub --daemon$"`)
		verificationSession, err := Start(verificationCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(verificationSession).Should(Exit(0))
	})

	It("does not return an error if a non-gpupgrade_hub process with gpupgrade_hub in the name is running", func() {
		path := filepath.Join(testWorkspaceDir, "gpupgrade_hub_test_log")
		f, err := os.Create(path)
		Expect(err).ToNot(HaveOccurred())
		f.Close()

		tailCmd := exec.Command("tail", "-f", path)
		tailSession, err := Start(tailCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer tailSession.Terminate()

		firstSession := runCommand("prepare", "start-hub")
		Expect(string(firstSession.Err.Contents())).Should(Equal(""))
		Eventually(firstSession).Should(Exit(0))
	})

	It("returns an error if gpupgrade_hub is already running", func() {
		firstSession := runCommand("prepare", "start-hub")
		Eventually(firstSession).Should(Exit(0))
		//second start should return error
		secondSession := runCommand("prepare", "start-hub")
		Eventually(secondSession).Should(Exit(1))
	})

	It("returns an error if gpupgrade_hub is not on the path", func() {
		origPath := os.Getenv("PATH")
		os.Setenv("PATH", "")
		gpUpgradeSession := runCommand("prepare", "start-hub")
		Eventually(gpUpgradeSession).ShouldNot(Exit(0))
		os.Setenv("PATH", origPath)
	})
})
