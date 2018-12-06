package utils_test

import (
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Version", func() {
	Describe("VersionString", func() {
		Context("when global var UpgradeVersion is the empty string", func() {
			It("returns the default version", func() {
				utils.UpgradeVersion = ""
				Expect(utils.VersionString("myname")).To(Equal("myname unknown version"))
			})
		})

		Context("when global var UpgradeVersion is set to something", func() {
			It("returns what it's set to", func() {
				utils.UpgradeVersion = "Something"
				Expect(utils.VersionString("gpupgrade")).To(Equal("gpupgrade version Something"))
			})
		})
	})
})
