package services

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"

	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/gomega"
)

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
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

func TestUpgradeMaster(t *testing.T) {
	g := NewGomegaWithT(t)

	// Disable exec.Command. This way, if a test forgets to mock it out, we
	// crash the test instead of executing code on a dev system.
	execCommand = nil

	// Initialize the sample cluster pair.
	pair := clusterPair{
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

	t.Run("sets the working directory", func(t *testing.T) {
		ctrl := gomock.NewController(t)
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
		g.Expect(err).NotTo(HaveOccurred())

		wd := buf.String()
		g.Expect(wd).To(Equal("/"))
	})

	t.Run("unsets PGPORT and PGHOST", func(t *testing.T) {
		ctrl := gomock.NewController(t)
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
		g.Expect(err).NotTo(HaveOccurred())

		scanner := bufio.NewScanner(&buf)
		for scanner.Scan() {
			g.Expect(scanner.Text()).NotTo(HavePrefix("PGPORT="),
				"PGPORT was not stripped from the child environment")
			g.Expect(scanner.Text()).NotTo(HavePrefix("PGHOST="),
				"PGHOST was not stripped from the child environment")
		}
		g.Expect(scanner.Err()).NotTo(HaveOccurred())
	})
}
