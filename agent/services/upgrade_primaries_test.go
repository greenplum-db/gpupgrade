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

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Does nothing.
func EmptyMain() {}

// Prints the environment, one variable per line, in NAME=VALUE format.
func EnvironmentMain() {
	for _, e := range os.Environ() {
		fmt.Println(e)
	}
}

func FailedMain() {
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		EmptyMain,
		EnvironmentMain,
		FailedMain,
	)
}

var _ = Describe("UpgradeSegments", func() {
	var (
		segments     []Segment
		sourceBinDir string
		targetBinDir string
		tmpDir       string
	)

	BeforeEach(func() {
		sourceBinDir = "/old/bin"
		targetBinDir = "/new/bin"

		var err error
		tmpDir, err = ioutil.TempDir("", "agenttest")
		Expect(err).ToNot(HaveOccurred())

		segments = []Segment{
			{
				WorkDir: tmpDir,
				DataDirPair: &idl.DataDirPair{
					OldDataDir: "old/datadir1",
					NewDataDir: "new/datadir1",
					Content:    0,
					OldPort:    1,
					NewPort:    11,
					DBID:       2,
				},
			},
			// TODO: Add a way to test multiple calls to execCommand.
			//{WorkDir: "", DataDirPair: &idl.DataDirPair{OldDataDir: "old/datadir2", NewDataDir: "new/datadir2", Content: 1, OldPort: 2, NewPort: 22}},
		}

		utils.System.MkdirAll = func(string, os.FileMode) error {
			return nil
		}

		// Disable exec.Command. This way, if a test forgets to mock it out, we
		// crash the test instead of executing code on a dev system.
		execCommand = nil
	})

	AfterEach(func() {
		execCommand = exec.Command

		err := os.RemoveAll(tmpDir)
		Expect(err).ToNot(HaveOccurred())
	})

	It("calls pg_upgrade with the expected options with no check", func() {
		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				// pg_upgrade should be run from the target installation.
				expectedPath := filepath.Join(targetBinDir, "pg_upgrade")
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
				oldDBID := fs.Int("old-gp-dbid", -1, "")
				newDBID := fs.Int("new-gp-dbid", -1, "")
				checkOnly := fs.Bool("check", false, "")
				retain := fs.Bool("retain", false, "")

				err := fs.Parse(args)
				Expect(err).NotTo(HaveOccurred())

				Expect(*oldBinDir).To(Equal(sourceBinDir))
				Expect(*newBinDir).To(Equal(targetBinDir))
				Expect(*oldDataDir).To(Equal(segments[0].OldDataDir))
				Expect(*newDataDir).To(Equal(segments[0].NewDataDir))
				Expect(*oldPort).To(Equal(int(segments[0].OldPort)))
				Expect(*newPort).To(Equal(int(segments[0].NewPort)))
				Expect(*mode).To(Equal("segment"))
				Expect(*oldDBID).To(Equal(int(segments[0].DBID)))
				Expect(*newDBID).To(Equal(int(segments[0].DBID)))
				Expect(*checkOnly).To(Equal(false))
				Expect(*retain).To(Equal(true))

				// No other arguments should be passed.
				Expect(fs.Args()).To(BeEmpty())
			})

		err := UpgradeSegments("/old/bin", "/new/bin", segments, false)
		Expect(err).NotTo(HaveOccurred())
	})

	It("calls pg_upgrade with the expected options with check", func() {
		execCommand = exectest.NewCommandWithVerifier(EmptyMain,
			func(path string, args ...string) {
				// pg_upgrade should be run from the target installation.
				expectedPath := filepath.Join(targetBinDir, "pg_upgrade")
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
				checkOnly := fs.Bool("check", false, "")
				oldDBID := fs.Int("old-gp-dbid", -1, "")
				newDBID := fs.Int("new-gp-dbid", -1, "")
				retain := fs.Bool("retain", false, "")

				err := fs.Parse(args)
				Expect(err).NotTo(HaveOccurred())

				Expect(*oldBinDir).To(Equal(sourceBinDir))
				Expect(*newBinDir).To(Equal(targetBinDir))
				Expect(*oldDataDir).To(Equal(segments[0].OldDataDir))
				Expect(*newDataDir).To(Equal(segments[0].NewDataDir))
				Expect(*oldPort).To(Equal(int(segments[0].OldPort)))
				Expect(*newPort).To(Equal(int(segments[0].NewPort)))
				Expect(*mode).To(Equal("segment"))
				Expect(*checkOnly).To(Equal(true))
				Expect(*oldDBID).To(Equal(int(segments[0].DBID)))
				Expect(*newDBID).To(Equal(int(segments[0].DBID)))
				Expect(*retain).To(Equal(true))

				// No other arguments should be passed.
				Expect(fs.Args()).To(BeEmpty())
			})

		err := UpgradeSegments("/old/bin", "/new/bin", segments, true)
		Expect(err).NotTo(HaveOccurred())
	})

	It("when pg_upgrade --check fails it returns an error", func() {
		execCommand = exectest.NewCommand(FailedMain)

		err := UpgradeSegments("/old/bin", "/new/bin", segments, true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to check primary on host"))
		Expect(err.Error()).To(ContainSubstring("with content 0"))
	})

	It("when pg_upgrade with no check fails it returns an error", func() {
		execCommand = exectest.NewCommand(FailedMain)

		err := UpgradeSegments("/old/bin", "/new/bin", segments, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to upgrade primary on host"))
		Expect(err.Error()).To(ContainSubstring("with content 0"))
	})

	// FIXME this test doesn't test anything; the system under test isn't
	// visible to the test implementation.
	It("unsets PGPORT and PGHOST", func() {
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
		err := UpgradeSegments("/old/bin", "/new/bin", segments, false)
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
