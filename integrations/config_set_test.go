package integrations_test

import (
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("config set", func() {
	It("fails if no flags are passed", func() {
		configSetSession := runCommand("config", "set")
		Expect(configSetSession).Should(Exit(1))
		Expect(string(configSetSession.Err.Contents())).To(ContainSubstring("the set command requires at least one flag to be specified"))
	})

	// XXX: Cobra handles this, so do we care to test this here?
	It("fails if an invalid flag is passed", func() {
		nonExistentFlag := "--no-existent-flag"
		configSetSession := runCommand("config", "set", nonExistentFlag, "foo")
		Expect(configSetSession).Should(Exit(1))
	})

	It("sets the source binary directory in the configuration file", func() {
		expected := "/source/bin/dir"
		configSetSession := runCommand("config", "set", "--old-bindir", expected)
		Expect(configSetSession).Should(Exit(0))

		source := &utils.Cluster{ConfigPath: filepath.Join(testStateDir, utils.SOURCE_CONFIG_FILENAME)}
		err := source.Load()
		Expect(err).To(BeNil())
		Expect(source.BinDir).To(Equal(expected))
	})

	It("sets the target binary directory in the configuration file", func() {
		expected := "/target/bin/dir"
		configSetSession := runCommand("config", "set", "--new-bindir", expected)
		Expect(configSetSession).Should(Exit(0))

		target := &utils.Cluster{ConfigPath: filepath.Join(testStateDir, utils.TARGET_CONFIG_FILENAME)}
		err := target.Load()
		Expect(err).To(BeNil())
		Expect(target.BinDir).To(Equal(expected))
	})
})
