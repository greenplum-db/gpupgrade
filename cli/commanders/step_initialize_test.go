package commanders

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	. "github.com/onsi/gomega"
)

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func IsHubRunning_True() {
	fmt.Print("1")
	os.Exit(0)
}

func IsHubRunning_False() {
	fmt.Print("0")
	os.Exit(1)
}

func IsHubRunning_Error() {
	fmt.Print("bengie")
	os.Exit(2)
}

func GpupgradeHub_good_Main() {
	fmt.Print("Hi, Hub started.")
}

func GpupgradeHub_bad_Main() {
	fmt.Fprint(os.Stderr, "Sorry, Hub could not be started.")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		IsHubRunning_True,
		IsHubRunning_False,
		IsHubRunning_Error,
		GpupgradeHub_good_Main,
		GpupgradeHub_bad_Main,
	)
}

var (
	g *GomegaWithT
)

func setup(t *testing.T) {
	g = NewGomegaWithT(t)
	execCommandHubStart = nil
	execCommandHubCount = nil
}

func teardown() {
	execCommandHubStart = exec.Command
	execCommandHubCount = exec.Command
}

func TestNoHubIsAlreadyRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_False)
	running, err := IsHubRunning()
	g.Expect(err).To(BeNil())
	g.Expect(running).To(BeFalse())
}

func TestHubIsAlreadyRunning(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_True)
	running, err := IsHubRunning()
	g.Expect(err).To(BeNil())
	g.Expect(running).To(BeTrue())
}

func TestHowManyHubsRunningFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubCount = exectest.NewCommand(IsHubRunning_Error)
	running, err := IsHubRunning()
	g.Expect(running).To(BeFalse())
	g.Expect(err).ToNot(BeNil())
}

func TestWeCanStartHub(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubStart = exectest.NewCommand(GpupgradeHub_good_Main)
	err := StartHub()
	g.Expect(err).To(BeNil())
}

func TestStartHubFails(t *testing.T) {
	setup(t)
	defer teardown()

	execCommandHubStart = exectest.NewCommand(GpupgradeHub_bad_Main)
	err := StartHub()
	g.Expect(err).ToNot(BeNil())
}
