package hub

import (
	"io"
	"io/ioutil"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

// Set it to nil so we don't accidentally execute a command for real during tests
func init() {
	ResetExecCommand()
	ResetRsyncExecCommand()
}

func SetExecCommand(cmdFunc exectest.Command) {
	execCommand = cmdFunc
}

func SetRsyncExecCommand(cmdFunc exectest.Command) {
	execCommandRsync = cmdFunc
}

func ResetExecCommand() {
	execCommand = nil
}

func ResetRsyncExecCommand() {
	execCommandRsync = nil
}

// DevNull implements OutStreams by just discarding all writes.
var DevNull = devNull{}

type devNull struct{}

func (_ devNull) Stdout() io.Writer {
	return ioutil.Discard
}

func (_ devNull) Stderr() io.Writer {
	return ioutil.Discard
}

// failingWriter is an io.Writer for which all calls to Write() return an error.
type failingWriter struct {
	err error
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}

// MustCreateCluster creates a utils.Cluster and calls t.Fatalf() if there is
// any error.
func MustCreateCluster(t *testing.T, segs []greenplum.SegConfig) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segs)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return cluster
}
