package services

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/cluster"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/gomega"
)

func EmptyMain() {}

func init() {
	exectest.RegisterMains(EmptyMain)
}

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
						DbID:    1,
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
						DbID:    2,
					},
				},
			},
		},
	}

	t.Run("sets the working directory", func(t *testing.T) {
		g := NewGomegaWithT(t)
		stream := new(bufferedStreams)

		// Print the working directory of the command.
		execCommand = exectest.NewCommand(WorkingDirectoryMain)

		// NOTE: avoid testing paths that might be symlinks, such as /tmp, as
		// the "actual" working directory might look different to the
		// subprocess.
		err := pair.ConvertMaster(stream, "/", false)
		g.Expect(err).NotTo(HaveOccurred())

		wd := stream.stdout.String()
		g.Expect(wd).To(Equal("/"))
	})

	t.Run("unsets PGPORT and PGHOST", func(t *testing.T) {
		g := NewGomegaWithT(t)
		stream := new(bufferedStreams)

		// Set our environment.
		os.Setenv("PGPORT", "5432")
		os.Setenv("PGHOST", "localhost")
		defer func() {
			os.Unsetenv("PGPORT")
			os.Unsetenv("PGHOST")
		}()

		// Echo the environment to stdout.
		execCommand = exectest.NewCommand(EnvironmentMain)

		err := pair.ConvertMaster(stream, "", false)
		g.Expect(err).NotTo(HaveOccurred())

		scanner := bufio.NewScanner(&stream.stdout)
		for scanner.Scan() {
			g.Expect(scanner.Text()).NotTo(HavePrefix("PGPORT="),
				"PGPORT was not stripped from the child environment")
			g.Expect(scanner.Text()).NotTo(HavePrefix("PGHOST="),
				"PGHOST was not stripped from the child environment")
		}
		g.Expect(scanner.Err()).NotTo(HaveOccurred())
	})

	t.Run("calls pg_upgrade with the expected options with no check", func(t *testing.T) {
		g := NewGomegaWithT(t)
		stream := new(bufferedStreams)

		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				// pg_upgrade should be run from the target installation.
				expectedPath := filepath.Join(pair.Target.BinDir, "pg_upgrade")
				g.Expect(path).To(Equal(expectedPath))

				// Check the arguments. We use a FlagSet so as not to couple
				// against option order.
				var fs flag.FlagSet

				oldBinDir := fs.String("old-bindir", "", "")
				newBinDir := fs.String("new-bindir", "", "")
				oldDataDir := fs.String("old-datadir", "", "")
				newDataDir := fs.String("new-datadir", "", "")
				oldPort := fs.Int("old-port", -1, "")
				newPort := fs.Int("new-port", -1, "")
				oldDBID := fs.Int("old-gp-dbid", -1, "")
				newDBID := fs.Int("new-gp-dbid", -1, "")
				mode := fs.String("mode", "", "")

				err := fs.Parse(args)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(*oldBinDir).To(Equal(pair.Source.BinDir))
				g.Expect(*newBinDir).To(Equal(pair.Target.BinDir))
				g.Expect(*oldDataDir).To(Equal(pair.Source.MasterDataDir()))
				g.Expect(*newDataDir).To(Equal(pair.Target.MasterDataDir()))
				g.Expect(*oldPort).To(Equal(pair.Source.MasterPort()))
				g.Expect(*newPort).To(Equal(pair.Target.MasterPort()))
				g.Expect(*oldDBID).To(Equal(pair.Source.GetDbidForContent(-1)))
				g.Expect(*newDBID).To(Equal(pair.Target.GetDbidForContent(-1)))
				g.Expect(*mode).To(Equal("dispatcher"))

				// No other arguments should be passed.
				g.Expect(fs.Args()).To(BeEmpty())
			})

		err := pair.ConvertMaster(stream, "", false)
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("calls pg_upgrade with the expected options with no check", func(t *testing.T) {
		g := NewGomegaWithT(t)
		stream := new(bufferedStreams)

		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				// pg_upgrade should be run from the target installation.
				expectedPath := filepath.Join(pair.Target.BinDir, "pg_upgrade")
				g.Expect(path).To(Equal(expectedPath))

				// Check the arguments. We use a FlagSet so as not to couple
				// against option order.
				var fs flag.FlagSet

				oldBinDir := fs.String("old-bindir", "", "")
				newBinDir := fs.String("new-bindir", "", "")
				oldDataDir := fs.String("old-datadir", "", "")
				newDataDir := fs.String("new-datadir", "", "")
				oldPort := fs.Int("old-port", -1, "")
				newPort := fs.Int("new-port", -1, "")
				oldDBID := fs.Int("old-gp-dbid", -1, "")
				newDBID := fs.Int("new-gp-dbid", -1, "")
				mode := fs.String("mode", "", "")
				checkOnly := fs.Bool("check", false, "")

				err := fs.Parse(args)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(*oldBinDir).To(Equal(pair.Source.BinDir))
				g.Expect(*newBinDir).To(Equal(pair.Target.BinDir))
				g.Expect(*oldDataDir).To(Equal(pair.Source.MasterDataDir()))
				g.Expect(*newDataDir).To(Equal(pair.Target.MasterDataDir()))
				g.Expect(*oldPort).To(Equal(pair.Source.MasterPort()))
				g.Expect(*newPort).To(Equal(pair.Target.MasterPort()))
				g.Expect(*oldDBID).To(Equal(pair.Source.GetDbidForContent(-1)))
				g.Expect(*newDBID).To(Equal(pair.Target.GetDbidForContent(-1)))
				g.Expect(*mode).To(Equal("dispatcher"))
				g.Expect(*checkOnly).To(Equal(true))

				// No other arguments should be passed.
				g.Expect(fs.Args()).To(BeEmpty())
			})

		err := pair.ConvertMaster(stream, "", true)
		g.Expect(err).NotTo(HaveOccurred())
	})
}
