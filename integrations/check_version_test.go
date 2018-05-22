package integrations_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("check version", func() {
	It("fails if the --master-host flag is missing", func() {
		checkVersionSession := runCommand("check", "version")
		Expect(checkVersionSession).Should(Exit(1))
		Expect(string(checkVersionSession.Out.Contents())).To(Equal("Required flag(s) \"master-host\" have/has not been set\n"))

	})
})
