package integrations_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	multierror "github.com/hashicorp/go-multierror"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Tests Suite")
}

var (
	testWorkspaceDir string // will be recreated for every test
	testStateDir     string // what would normally be ~/.gpupgrade
)

const (
	cliToHubPort = 7527
)

var _ = BeforeSuite(func() {
	// All gpupgrade binaries are expected to be on the path for integration
	// tests. Be nice to developers and check up front; warn if the binaries
	// being tested aren't contained in a directory directly above this test
	// file.
	_, testPath, _, ok := runtime.Caller(0)
	if !ok {
		Fail("couldn't retrieve Caller() information")
	}

	var allErrs error
	for _, bin := range []string{"gpupgrade"} {
		binPath, err := exec.LookPath(bin)
		if err != nil {
			allErrs = multierror.Append(allErrs, err)
			continue
		}

		dir := filepath.Dir(binPath)
		if !strings.HasPrefix(testPath, dir) {
			log.Printf("warning: tested binary %s doesn't appear to be locally built", binPath)
		}
	}
	if allErrs != nil {
		Fail(fmt.Sprintf(
			"Please put gpupgrade binaries on your PATH before running integration tests.\n%s",
			multierror.Flatten(allErrs),
		))
	}
})

// BeforeEach for the integrations suite will create the testWorkspaceDir and
// testStateDir for tests. GPUPGRADE_HOME is set to the testStateDir, so tests
// may run the hub without worrying about colliding with the developer
// environment. Both directories are removed in the suite AfterEach.
var _ = BeforeEach(func() {
	var err error
	testWorkspaceDir, err = ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())
	testStateDir = filepath.Join(testWorkspaceDir, ".gpupgrade")
	os.Setenv("GPUPGRADE_HOME", testStateDir)
})

var _ = AfterEach(func() {
	os.RemoveAll(testWorkspaceDir)
})

// killHub finds all running hub processes and kills them.
// XXX we should really use a PID file for this, and allow side-by-side hubs,
// rather than blowing away developer state.
func killHub() {
	killCommand := exec.Command("pkill", "-f", "^gpupgrade hub")
	err := killCommand.Run()

	// pkill returns exit code 1 if no processes were matched, which is fine.
	if err != nil {
		Expect(err).To(MatchError("exit status 1"))
	} else {
		Expect(err).ToNot(HaveOccurred())
	}

	Expect(checkPortIsAvailable(cliToHubPort)).To(BeTrue())
}

func checkPortIsAvailable(port int) bool {
	t := time.After(2 * time.Second)
	select {
	case <-t:
		fmt.Println("timed out")
		break
	default:
		cmd := exec.Command("/bin/sh", "-c", "'lsof | grep "+strconv.Itoa(port)+"'")
		err := cmd.Run()
		output, _ := cmd.CombinedOutput()
		if _, ok := err.(*exec.ExitError); ok && string(output) == "" {
			return true
		}

		time.Sleep(250 * time.Millisecond)
	}

	return false
}
