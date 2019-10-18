package services

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const StreamingMainStdout = "expected\nstdout\n"
const StreamingMainStderr = "process\nstderr\n"

// Streams the above stdout/err constants to the corresponding standard file
// descriptors, alternately interleaving five-byte chunks.
func StreamingMain() {
	stdout := bytes.NewBufferString(StreamingMainStdout)
	stderr := bytes.NewBufferString(StreamingMainStderr)

	for stdout.Len() > 0 || stderr.Len() > 0 {
		os.Stdout.Write(stdout.Next(5))
		os.Stderr.Write(stderr.Next(5))
	}
}

// Streams exactly ten bytes ('O' on stdout and 'E' on stderr) per standard
// stream.
func TenByteMain() {
	for i := 0; i < 10; i++ {
		os.Stdout.Write([]byte{'O'})
		os.Stderr.Write([]byte{'E'})
	}
}

// Writes to stdout and ignores any failure to do so.
func BlindlyWritingMain() {
	// Ignore SIGPIPE. Note that the obvious signal.Ignore(syscall.SIGPIPE)
	// doesn't work as expected; see https://github.com/golang/go/issues/32386.
	signal.Notify(make(chan os.Signal), syscall.SIGPIPE)

	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
	fmt.Println("blah blah blah blah")
}

func init() {
	exectest.RegisterMains(
		StreamingMain,
		TenByteMain,
		BlindlyWritingMain,
	)
}

// NewFailingWriter creates an io.Writer that will fail with the given error.
func NewFailingWriter(err error) io.Writer {
	return &failingWriter{
		err: err,
	}
}

type failingWriter struct {
	err error
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}

func TestStreaming(t *testing.T) {
	g := NewGomegaWithT(t)

	var pair clusterPair   // the unit under test
	var log *gbytes.Buffer // contains gplog output

	// Disable exec.Command. This way, if a test forgets to mock it out, we
	// crash the test instead of executing code on a dev system.
	execCommand = nil

	// Store gplog output.
	_, _, log = testhelper.SetupTestLogger()

	// Initialize the sample cluster pair.
	pair = clusterPair{
		Source: &utils.Cluster{
			BinDir: "/old/bin",
			Cluster: &cluster.Cluster{
				ContentIDs: []int{-1},
				Segments: map[int]cluster.SegConfig{
					-1: cluster.SegConfig{
						Port:    5432,
						DataDir: "/data/old",
					},
				},
			},
		},
		Target: &utils.Cluster{
			BinDir: "/new/bin",
			Cluster: &cluster.Cluster{
				ContentIDs: []int{-1},
				Segments: map[int]cluster.SegConfig{
					-1: cluster.SegConfig{
						Port:    5433,
						DataDir: "/data/new",
					},
				},
			},
		},
	}

	t.Run("streams stdout and stderr to the client", func(t *testing.T) {
		// We can't rely on each write from the subprocess to result in exactly
		// one call to stream.Send(). Instead, concatenate the byte buffers as
		// they are sent and compare them at the end.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes(). // Send will be called an indeterminate number of times

			DoAndReturn(func(msg *idl.Message) error {
				defer GinkgoRecover()

				var buf *bytes.Buffer
				c := msg.GetChunk()

				switch c.Type {
				case idl.Chunk_STDOUT:
					buf = &stdout
				case idl.Chunk_STDERR:
					buf = &stderr
				default:
					Fail("unexpected chunk type")
				}

				buf.Write(c.Buffer)
				return nil
			})

		execCommand = exectest.NewCommand(StreamingMain)

		err := pair.ConvertMaster(mockStream, ioutil.Discard, "")
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(stdout.String()).To(Equal(StreamingMainStdout))
		g.Expect(stderr.String()).To(Equal(StreamingMainStderr))
	})

	t.Run("also writes all data to a local io.Writer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Write ten bytes each to stdout/err.
		execCommand = exectest.NewCommand(TenByteMain)

		var buf bytes.Buffer
		err := pair.ConvertMaster(mockStream, &buf, "")
		g.Expect(err).NotTo(HaveOccurred())

		// Stdout and stderr are not guaranteed to interleave in any particular
		// order. Just count the number of bytes in each that we see (there
		// should be exactly ten).
		numO := 0
		numE := 0
		for _, b := range buf.Bytes() {
			switch b {
			case 'O':
				numO++
			case 'E':
				numE++
			default:
				Fail(fmt.Sprintf("unexpected byte %#v in output %#v", b, buf.String()))
			}
		}

		g.Expect(numO).To(Equal(10))
		g.Expect(numE).To(Equal(10))
	})

	t.Run("returns an error if the command succeeds but the io.Writer fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Don't fail in the subprocess even when the stdout stream is closed.
		execCommand = exectest.NewCommand(BlindlyWritingMain)

		expectedErr := errors.New("write failed!")
		err := pair.ConvertMaster(mockStream, NewFailingWriter(expectedErr), "")

		g.Expect(err).To(Equal(expectedErr))
	})

	t.Run("continues writing to the local io.Writer even if Send fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Return an error during Send.
		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			Return(errors.New("error during send")).
			Times(1) // we expect only one failed attempt to Send

		// Write ten bytes each to stdout/err.
		execCommand = exectest.NewCommand(TenByteMain)

		var buf bytes.Buffer
		err := pair.ConvertMaster(mockStream, &buf, "")
		g.Expect(err).NotTo(HaveOccurred())

		// The Writer should not have been affected in any way.
		g.Expect(buf.Bytes()).To(HaveLen(20))
		g.Expect(log).To(gbytes.Say("halting client stream: error during send"))
	})
}
