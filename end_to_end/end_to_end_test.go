package end_to_end_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/iohelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("gpupgrade end to end tests", func() {
	Context("Flag and subcommand validation", func() {
		It("outputs help text when no params are given", func() {
			output, err := gpupgrade()
			Expect(err).To(HaveOccurred())
			Expect(output).To(ContainSubstring("Please specify one command of: check, prepare, status, upgrade, or version"))
		})
		DescribeTable("gpupgrade fails when",
			func(command string, subcommand string, expectedFlags ...string) {
				output, err := gpupgrade(command, subcommand)
				flagStr := strings.Join(expectedFlags, `", "`)
				Expect(err).To(HaveOccurred())
				Expect(output).To(ContainSubstring(fmt.Sprintf(`Error: Required flag(s) "%s" have/has not been set`, flagStr)))
			},
			Entry("check config is missing its flags", "check", "config", "master-host", "old-bindir"),
			Entry("check disk-space is missing its flag", "check", "disk-space", "master-host"),
			Entry("check object-count is missing its flag", "check", "object-count", "master-host"),
			Entry("check seginstall is missing its flag", "check", "seginstall", "master-host"),
			Entry("check version is missing its flag", "check", "version", "master-host"),
			Entry("prepare init-cluster is missing its flags", "prepare", "init-cluster", "new-bindir", "port"),
			Entry("prepare shutdown-clusters is missing its flags", "prepare", "shutdown-clusters", "new-bindir", "old-bindir"),
			Entry("upgrade convert-master is missing its flags", "upgrade", "convert-master", "new-bindir", "new-datadir", "old-bindir", "old-datadir"),
			Entry("upgrade convert-primaries is missing its flags", "upgrade", "convert-primaries", "new-bindir", "old-bindir"),
		)
	})
	Context("prepare start-hub", func() {
		It("finds the right hub binary and starts a daemonized process", func() {
			_, err := gpupgrade("prepare", "start-hub")
			Expect(err).ToNot(HaveOccurred())
			running := verifyProcessIsRunning("gpupgrade_hub --daemon")
			Expect(running).To(BeTrue())
		})
		It("returns an error if the hub is already running", func() {
			_, err := gpupgrade("prepare", "start-hub")
			Expect(err).ToNot(HaveOccurred())
			output, err := gpupgrade("prepare", "start-hub")
			// TODO: start-hub needs a better error message for this case
			Expect(output).To(ContainSubstring("bind: address already in use: failed to listen"))
			Expect(err).To(HaveOccurred())
		})
		It("does not return an error if an unrelated process has gpupgrade_hub in its name", func() {
			testWorkspaceDir, err := ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())
			logPath := filepath.Join(testWorkspaceDir, "gpupgrade_hub_test_log")
			f := iohelper.MustOpenFileForWriting(logPath)
			f.Close()

			tailCmd := exec.Command("tail", "-f", logPath)
			tailSession, err := gexec.Start(tailCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			defer tailSession.Terminate()

			_, err = gpupgrade("prepare", "start-hub")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if gpupgrade_hub isn't on the PATH", func() {
			origPath := os.Getenv("PATH")
			os.Setenv("PATH", "")
			defer os.Setenv("PATH", origPath)
			_, err := gpupgrade("prepare", "start-hub")
			Expect(err).To(HaveOccurred())
		})
	})
	Context("prepare start-agents", func() {
		It("returns an error if the hub is not started", func() {
			output, err := gpupgrade("prepare", "start-agents")
			Expect(err).To(HaveOccurred())
			Expect(output).To(ContainSubstring("couldn't connect to the upgrade hub (did you run 'gpupgrade prepare start-hub'?)"))
		})
	})
})
