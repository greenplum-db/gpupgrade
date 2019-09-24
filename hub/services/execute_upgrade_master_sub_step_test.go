package services

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

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
		EmptyMain,
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

var _ = Describe("ConvertMaster", func() {
	var pair clusterPair   // the unit under test

	BeforeEach(func() {
		// Disable exec.Command. This way, if a test forgets to mock it out, we
		// crash the test instead of executing code on a dev system.
		execCommand = nil

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
