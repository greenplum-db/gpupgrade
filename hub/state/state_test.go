package state

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestSaveConfig(t *testing.T) {
	source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
	useLinkMode := false

	state := State{
		StateDir: "",
		Config: &Config{
			Source:                 source,
			Target:                 target,
			TargetInitializeConfig: InitializeConfig{},
			Port:                   12345,
			AgentPort:              54321,
			UseLinkMode:            useLinkMode,
		},
	}

	t.Run("saves configuration contents to disk", func(t *testing.T) {
		// Set up utils.System.Create to return the write side of a pipe. We can
		// read from the other side to confirm what was saved to "disk".
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatalf("creating pipe: %+v", err)
		}
		defer func() {
			read.Close()
			write.Close()
		}()

		utils.System.Create = func(path string) (*os.File, error) {
			return write, nil
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		// Write the hub's configuration to the pipe.
		if err := state.Save(); err != nil {
			t.Errorf("save() returned error %+v", err)
		}

		// Reload the configuration from the read side of the pipe and ensure the
		// contents are the same.
		actual := new(Config)
		if err := actual.load(read); err != nil {
			t.Errorf("loading configuration results: %+v", err)
		}

		if !reflect.DeepEqual(state.Config, actual) {
			t.Errorf("wrote config %#v, want %#v", actual, state.Config)
		}
	})

	t.Run("bubbles up file creation errors", func(t *testing.T) {
		expected := errors.New("can't create")

		utils.System.Create = func(path string) (*os.File, error) {
			return nil, expected
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := state.Save()
		if !xerrors.Is(err, expected) {
			t.Errorf("returned %#v, want %#v", err, expected)
		}
	})

	t.Run("bubbles up file manipulation errors", func(t *testing.T) {
		// A nil file will fail to write and close, so we can make sure things
		// are handled correctly.
		utils.System.Create = func(path string) (*os.File, error) {
			return nil, nil
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := state.Save()

		// multierror.Error that contains os.ErrInvalid is not itself an instance
		// of os.ErrInvalid, so unpack it to check existence of os.ErrInvalid
		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}

		for _, err := range merr.Errors {
			// For nil Files, operations return os.ErrInvalid.
			if !xerrors.Is(err, os.ErrInvalid) {
				t.Errorf("returned error %#v, want %#v", err, os.ErrInvalid)
			}
		}
	})
}
