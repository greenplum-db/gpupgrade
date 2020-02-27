package integrations_test

import (
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
			command := command
			flag := flag
			help := help

			It(fmt.Sprintf("testing command %q with flag %q", command, flag), func() {
				cmd := exec.Command("gpupgrade", command, flag)
				if command == "" {
					cmd = exec.Command("gpupgrade", flag)
				}
				output, err := cmd.Output()
				Expect(err).ToNot(HaveOccurred())
				Expect(string(output)).To(Equal(help))
			})
		}
	}

	It("testing command gpupgrade with no arguments", func() {
		cmd := exec.Command("gpupgrade")
		output, err := cmd.Output()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(output)).To(Equal(commands.GlobalHelp))
	})

})
