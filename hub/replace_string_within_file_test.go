package hub_test

import (
	"io/ioutil"
	"os/exec"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
)

func TestReplaceStringWithinFile(t *testing.T) {
	t.Run("it can replace a string within a file", func(t *testing.T) {
		testhelper.SetupTestLogger()

		hub.SetExecCommand(exec.Command)
		defer func() { hub.SetExecCommand(nil) }()

		hub.ResetReplaceStringWithinFile()

		file, err := ioutil.TempFile("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = file.WriteString("hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = file.Close()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		filePath := file.Name()

		err = hub.ReplaceStringWithinFile("hello", "goodbye", filePath)

		if err != nil {
			t.Fatalf("unexpected error: %q", err)
		}

		bytes, err := ioutil.ReadFile(filePath)

		actualString := string(bytes)
		if actualString != "goodbye\n" {
			t.Errorf("got %q, expected %q", actualString, "goodbye\n")
		}
	})
}
