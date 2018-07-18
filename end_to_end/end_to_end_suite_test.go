package end_to_end_test

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "End-to-End Suite")
}

var (
	gpupgradePath string
)

func gpupgrade(args ...string) (string, error) {
	command := exec.Command(gpupgradePath, args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func buildAndInstallBinaries() string {
	os.Chdir("..")
	command := exec.Command("make", "install")
	output, err := command.CombinedOutput()
	if err != nil {
		fmt.Printf("%s", output)
		Fail(fmt.Sprintf("Failed to install binaries: %v", err))
	}
	os.Chdir("end_to_end")
	binDir := fmt.Sprintf("%s/go/bin", operating.System.Getenv("HOME"))
	return fmt.Sprintf("%s/gpupgrade", binDir)
}

func killAll() {
	exec.Command("pkill", "-9", "gpupgrade_*").Run()
}

func verifyProcessIsRunning(process string) bool {
	// Change e.g. "gpupgrade" to "[g]pupgrade" so grep doesn't find itself
	psString := fmt.Sprintf(`ps -ef | grep -Gq "[%s]%s$"`, string(process[0]), process[1:])
	err := exec.Command("bash", "-c", psString).Run()
	return err == nil
}

var _ = BeforeSuite(func() {
	testhelper.SetupTestLogger()
	killAll()
	gpupgradePath = buildAndInstallBinaries()
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

var _ = AfterEach(func() {
	killAll()
})
