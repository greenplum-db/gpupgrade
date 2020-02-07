package hub

import (
	"io"
	"io/ioutil"

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

// failingStreams is an implementation of OutStreams for which every call to a
// stream's Write() method will fail with the given error.
type failingStreams struct {
	err error
}

func (f failingStreams) Stdout() io.Writer {
	return &failingWriter{f.err}
}

func (f failingStreams) Stderr() io.Writer {
	return &failingWriter{f.err}
}

// failingWriter is an io.Writer for which all calls to Write() return an error.
type failingWriter struct {
	err error
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}
