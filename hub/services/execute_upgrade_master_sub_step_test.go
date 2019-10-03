package services

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
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

// Does nothing.
func EmptyMain() {}

// Writes the current working directory to stdout.
func WorkingDirectoryMain() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v", err)
		os.Exit(1)
	}

	fmt.Print(wd)
}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func init() {
	exectest.RegisterMains(
		StreamingMain,
		TenByteMain,
		BlindlyWritingMain,
		EmptyMain,
		WorkingDirectoryMain,
		EnvironmentMain,
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

var _ = Describe("ConvertMaster", func() {
	var pair clusterPair   // the unit under test
	var log *gbytes.Buffer // contains gplog output

	BeforeEach(func() {
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
	})

	AfterEach(func() {
		execCommand = exec.Command
	})

	It("streams stdout and stderr to the client", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		// We can't rely on each write from the subprocess to result in exactly
		// one call to stream.Send(). Instead, concatenate the byte buffers as
		// they are sent and compare them at the end.
		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes(). // Send will be called an indeterminate number of times

			DoAndReturn(func(msg *idl.ExecuteMessage) error {
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
		Expect(err).NotTo(HaveOccurred())

		Expect(stdout.String()).To(Equal(StreamingMainStdout))
		Expect(stderr.String()).To(Equal(StreamingMainStderr))
	})

	It("also writes all data to a local io.Writer", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Write ten bytes each to stdout/err.
		execCommand = exectest.NewCommand(TenByteMain)

		var buf bytes.Buffer
		err := pair.ConvertMaster(mockStream, &buf, "")
		Expect(err).NotTo(HaveOccurred())

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

		Expect(numO).To(Equal(10))
		Expect(numE).To(Equal(10))
	})

	It("returns an error if the command succeeds but the io.Writer fails", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Don't fail in the subprocess even when the stdout stream is closed.
		execCommand = exectest.NewCommand(BlindlyWritingMain)

		expectedErr := errors.New("write failed!")
		err := pair.ConvertMaster(mockStream, NewFailingWriter(expectedErr), "")

		Expect(err).To(Equal(expectedErr))
	})

	It("continues writing to the local io.Writer even if Send fails", func() {
		ctrl := gomock.NewController(GinkgoT())
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
		Expect(err).NotTo(HaveOccurred())

		// The Writer should not have been affected in any way.
		Expect(buf.Bytes()).To(HaveLen(20))
		Expect(log).To(gbytes.Say("halting client stream: error during send"))
	})

	It("calls pg_upgrade with the expected options", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				// pg_upgrade should be run from the target installation.
				expectedPath := filepath.Join(pair.Target.BinDir, "pg_upgrade")
				Expect(path).To(Equal(expectedPath))

				// Check the arguments. We use a FlagSet so as not to couple
				// against option order.
				var fs flag.FlagSet

				oldBinDir := fs.String("old-bindir", "", "")
				newBinDir := fs.String("new-bindir", "", "")
				oldDataDir := fs.String("old-datadir", "", "")
				newDataDir := fs.String("new-datadir", "", "")
				oldPort := fs.Int("old-port", -1, "")
				newPort := fs.Int("new-port", -1, "")
				mode := fs.String("mode", "", "")

				err := fs.Parse(args)
				Expect(err).NotTo(HaveOccurred())

				Expect(*oldBinDir).To(Equal(pair.Source.BinDir))
				Expect(*newBinDir).To(Equal(pair.Target.BinDir))
				Expect(*oldDataDir).To(Equal(pair.Source.MasterDataDir()))
				Expect(*newDataDir).To(Equal(pair.Target.MasterDataDir()))
				Expect(*oldPort).To(Equal(pair.Source.MasterPort()))
				Expect(*newPort).To(Equal(pair.Target.MasterPort()))
				Expect(*mode).To(Equal("dispatcher"))

				// No other arguments should be passed.
				Expect(fs.Args()).To(BeEmpty())
			})

		err := pair.ConvertMaster(mockStream, ioutil.Discard, "")
		Expect(err).NotTo(HaveOccurred())
	})

	It("sets the working directory", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Print the working directory of the command.
		execCommand = exectest.NewCommand(WorkingDirectoryMain)

		// NOTE: avoid testing paths that might be symlinks, such as /tmp, as
		// the "actual" working directory might look different to the
		// subprocess.
		var buf bytes.Buffer
		err := pair.ConvertMaster(mockStream, &buf, "/")
		Expect(err).NotTo(HaveOccurred())

		wd := buf.String()
		Expect(wd).To(Equal("/"))
	})

	It("unsets PGPORT and PGHOST", func() {
		ctrl := gomock.NewController(GinkgoT())
		defer ctrl.Finish()

		mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		mockStream.EXPECT().
			Send(gomock.Any()).
			AnyTimes()

		// Set our environment.
		os.Setenv("PGPORT", "5432")
		os.Setenv("PGHOST", "localhost")
		defer func() {
			os.Unsetenv("PGPORT")
			os.Unsetenv("PGHOST")
		}()

		// Echo the environment to stdout.
		execCommand = exectest.NewCommand(EnvironmentMain)

		var buf bytes.Buffer
		err := pair.ConvertMaster(mockStream, &buf, "")
		Expect(err).NotTo(HaveOccurred())

		scanner := bufio.NewScanner(&buf)
		for scanner.Scan() {
			Expect(scanner.Text()).NotTo(HavePrefix("PGPORT="),
				"PGPORT was not stripped from the child environment")
			Expect(scanner.Text()).NotTo(HavePrefix("PGHOST="),
				"PGHOST was not stripped from the child environment")
		}
		Expect(scanner.Err()).NotTo(HaveOccurred())
	})
})
