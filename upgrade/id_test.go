package upgrade_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gpupgrade/upgrade"
)

func TestID(t *testing.T) {
	t.Run("NewID gives a unique identifier for each run", func(t *testing.T) {
		one := upgrade.NewID()
		two := upgrade.NewID()

		if one == two {
			t.Errorf("second generated ID was equal to first ID (%d)", one)
		}
	})

	t.Run("String gives a base64 representation of the ID", func(t *testing.T) {
		var id upgrade.ID

		expected := "AAAAAAAAAAA" // all zeroes in base64. 8 bytes decoded -> 11 bytes encoded
		if id.String() != expected {
			t.Errorf("String() returned %q, want %q", id.String(), expected)
		}
	})
}

// TestNewIDCrossProcess ensures that NewID returns different results across
// invocations of an executable (i.e. that the ID source is seeded correctly).
func TestNewIDCrossProcess(t *testing.T) {
	cmd1 := idCommand()
	cmd2 := idCommand()

	out1, err := cmd1.Output()
	if err != nil {
		t.Errorf("first execution: unexpected error %+v", err)
	}

	out2, err := cmd2.Output()
	if err != nil {
		t.Errorf("second execution: unexpected error %+v", err)
	}

	if string(out1) == string(out2) {
		t.Errorf("second generated ID was equal to first ID (%s)", string(out1))
	}
}

// idCommand creates an exec.Cmd that will run upgrade.NewID() in a brand-new
// process. It uses the TestIDCommand entry point to do its work.
func idCommand() *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestIDCommand")
	cmd.Env = append(cmd.Env, "GO_RUN_NEW_ID=1")
	return cmd
}

// TestIDCommand is the entry point for the idCommand(). It simply prints the
// result of an upgrade.NewID().
func TestIDCommand(_ *testing.T) {
	if os.Getenv("GO_RUN_NEW_ID") != "1" {
		return
	}

	fmt.Printf("%d", upgrade.NewID())
	os.Exit(0)
}
