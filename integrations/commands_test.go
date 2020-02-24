package integrations_test

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/greenplum-db/gpupgrade/cli/commands"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("test gpupgrade help messages", func() {
	helpMap := map[string]string{
		"":           commands.GlobalHelp,
		"initialize": commands.InitializeHelp,
		"execute":    commands.ExecuteHelp,
		"finalize":   commands.FinalizeHelp,
	}
	flagList := []string{"-?", "-h", "--help", "help"}

	for command, help := range helpMap {
		for _, flag := range flagList {
			It(fmt.Sprintf("testing command %s with flag %s", command, flag), func() {
				cmd := exec.Command("gpupgrade", command, flag)
				output, err := cmd.Output()
				Expect(err).ToNot(HaveOccurred())
				Expect(bytes.Compare(output, []byte(help))).To(BeZero())
			})
		}
	}
})
