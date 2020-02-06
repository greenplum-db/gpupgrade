package commands_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/greenplum-db/gpupgrade/cli/commands"
)

var _ = Describe("Version", func() {
	Describe("VersionString", func() {
		Context("when global var UpgradeVersion is the empty string", func() {
			It("returns the default version", func() {
				commands.UpgradeVersion = ""
				Expect(commands.VersionString("myname")).To(Equal("myname unknown version"))
			})
		})

		Context("when global var UpgradeVersion is set to something", func() {
			It("returns what it's set to", func() {
				commands.UpgradeVersion = "Something"
				Expect(commands.VersionString("gpupgrade")).To(Equal("gpupgrade version Something"))
			})
		})
	})
})
