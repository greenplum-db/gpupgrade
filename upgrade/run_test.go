// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func Success() {}
func Failure() { os.Exit(1) }

// Prints the strings "stdout" and "stderr" to the respective streams.
func PrintMain() {
	fmt.Fprint(os.Stdout, "stdout")
	fmt.Fprint(os.Stderr, "stderr")
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
		Success,
		Failure,
		PrintMain,
		WorkingDirectoryMain,
		EnvironmentMain,
	)
}

func TestRun(t *testing.T) {
	testlog.SetupLogger()

	pair := upgrade.SegmentPair{
		Source: &upgrade.Segment{
			BinDir:  "/old/bin",
			DataDir: "/old/data",
			DBID:    10,
			Port:    15432,
		},
		Target: &upgrade.Segment{
			BinDir:  "/new/bin",
			DataDir: "/new/data",
			DBID:    20,
			Port:    15433,
		},
	}

	// For our simplest tests, just call Run() with the desired options and fail
	// if there's an error.
	test := func(t *testing.T, cmd exectest.Command, opts ...upgrade.Option) {
		t.Helper()

		upgrade.SetExecCommand(cmd)
		defer upgrade.ResetExecCommand()

		err := upgrade.Run(pair, opts...)
		if err != nil {
			t.Errorf("Run() returned error %+v", err)
		}
	}

	t.Run("finds pg_upgrade in the target binary directory", func(t *testing.T) {
		var called bool

		cmd := exectest.NewCommandWithVerifier(Success, func(path string, _ ...string) {
			called = true

			expected := filepath.Join(pair.Target.BinDir, "pg_upgrade")
			if path != expected {
				t.Errorf("executed %q, want %q", path, expected)
			}
		})

		test(t, cmd)

		if !called {
			t.Errorf("pg_upgrade was not executed")
		}
	})

	t.Run("can control output destinations", func(t *testing.T) {
		cmd := exectest.NewCommand(PrintMain)

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		test(t, cmd, upgrade.WithOutputStreams(stdout, stderr))

		actual := stdout.String()
		if actual != "stdout" {
			t.Errorf("stdout contents were %q, want %q", actual, "stdout")
		}

		actual = stderr.String()
		if actual != "stderr" {
			t.Errorf("stderr contents were %q, want %q", actual, "stderr")
		}
	})

	t.Run("can set the working directory", func(t *testing.T) {
		// Print the working directory of the command.
		cmd := exectest.NewCommand(WorkingDirectoryMain)

		// NOTE: avoid testing paths that might be symlinks, such as /tmp, as
		// the "actual" working directory might look different to the
		// subprocess.
		wd := "/"
		stdout := new(bytes.Buffer)

		test(t, cmd,
			upgrade.WithOutputStreams(stdout, nil), upgrade.WithWorkDir(wd))

		actual := stdout.String()
		if actual != wd {
			t.Errorf("working directory was %q, want %q", actual, wd)
		}
	})

	t.Run("unsets PGPORT and PGHOST", func(t *testing.T) {
		// Set our environment.
		resetPort := testutils.SetEnv(t, "PGPORT", "5432")
		defer resetPort()

		resetHost := testutils.SetEnv(t, "PGHOST", "localhost")
		defer resetHost()

		// Echo the environment to stdout.
		cmd := exectest.NewCommand(EnvironmentMain)
		stdout := new(bytes.Buffer)

		test(t, cmd, upgrade.WithOutputStreams(stdout, nil))

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "PGPORT=") {
				t.Error("PGPORT was not stripped from the child environment")
			}
			if strings.HasPrefix(line, "PGHOST=") {
				t.Error("PGHOST was not stripped from the child environment")
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("got error during scan: %+v", err)
		}

	})

	t.Run("sets __GPDB_PGUPGRADE_PRINT_TIMING__", func(t *testing.T) {
		printTiming := "__GPDB_PGUPGRADE_PRINT_TIMING__"
		resetEnv := testutils.MustClearEnv(t, printTiming)
		defer resetEnv()

		// Echo the environment to stdout and to a copy for debugging
		cmd := exectest.NewCommand(EnvironmentMain)
		stdout := new(bytes.Buffer)

		test(t, cmd, upgrade.WithOutputStreams(stdout, nil))
		t.Logf("stdout was:\n%s", stdout)

		// search for printTiming in the environment variables
		var hasPrintTiming bool
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, printTiming) {
				hasPrintTiming = true
				break
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("got error during scan: %+v", err)
		}
		if !hasPrintTiming {
			t.Errorf("expected stdout to contain %q", printTiming)
		}

	})

	t.Run("can inject a caller-defined Command stub", func(t *testing.T) {
		// Note that we expect this Command implementation NOT to be used; the
		// WithExecCommand() option should override it.
		cmd := exectest.NewCommand(Failure)

		// The test succeeds if upgrade.Run() doesn't return an error.
		test(t, cmd, upgrade.WithExecCommand(exectest.NewCommand(Success)))
	})

	t.Run("bubbles up any errors", func(t *testing.T) {
		upgrade.SetExecCommand(exectest.NewCommand(Failure))
		defer upgrade.ResetExecCommand()

		err := upgrade.Run(pair)

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("got error %#v, want type *exec.ExitError", err)
		}

		if exitErr.ExitCode() != 1 {
			t.Errorf("got exit code %d, want 1", exitErr.ExitCode())
		}
	})

	t.Run("calls pg_upgrade with the correct arguments for", func(t *testing.T) {
		argsTest := func(t *testing.T, opts ...upgrade.Option) {
			t.Helper()

			options := upgrade.NewOptionList(opts)

			mode := "dispatcher"
			if options.SegmentMode {
				mode = "segment"
			}

			cmd := exectest.NewCommandWithVerifier(Success, func(_ string, args ...string) {
				// Check the arguments. We use a FlagSet so as not to couple
				// against option order.
				var fs flag.FlagSet

				fs.String("old-bindir", "", "")
				fs.String("new-bindir", "", "")
				fs.Int("old-gp-dbid", -1, "")
				fs.Int("new-gp-dbid", -1, "")
				fs.String("old-datadir", "", "")
				fs.String("new-datadir", "", "")
				fs.Int("old-port", -1, "")
				fs.Int("new-port", -1, "")
				fs.String("mode", "", "")
				fs.Bool("check", false, "")
				fs.Bool("retain", false, "")
				fs.Bool("link", false, "")
				fs.String("old-tablespaces-file", "", "")
				fs.String("old-options", "", "")

				err := fs.Parse(args)
				if err != nil {
					t.Fatalf("error parsing arguments: %+v", err)
				}

				expected := map[string]interface{}{
					"old-bindir":           pair.Source.BinDir,
					"new-bindir":           pair.Target.BinDir,
					"old-gp-dbid":          pair.Source.DBID,
					"new-gp-dbid":          pair.Target.DBID,
					"old-datadir":          pair.Source.DataDir,
					"new-datadir":          pair.Target.DataDir,
					"old-port":             pair.Source.Port,
					"new-port":             pair.Target.Port,
					"mode":                 mode,
					"check":                options.CheckOnly,
					"retain":               true,
					"link":                 options.UseLinkMode,
					"old-tablespaces-file": options.TablespaceFilePath,
					"old-options":          options.OldOptions,
				}

				fs.VisitAll(func(f *flag.Flag) {
					value := f.Value.(flag.Getter)

					if value.Get() != expected[f.Name] {
						t.Errorf("got --%s %#v, want %#v", f.Name, value.Get(), expected[f.Name])
					}
				})

				// No other arguments should be passed.
				if len(fs.Args()) != 0 {
					t.Errorf("got unexpected arguments %q", fs.Args())
				}
			})

			test(t, cmd, opts...)
		}

		cases := []struct {
			name    string
			options []upgrade.Option
		}{
			{"the master (default)", []upgrade.Option{}},
			{"segments", []upgrade.Option{upgrade.WithSegmentMode()}},
			{"--check mode on master", []upgrade.Option{upgrade.WithCheckOnly()}},
			{"--check mode on segments", []upgrade.Option{upgrade.WithSegmentMode(), upgrade.WithCheckOnly()}},
			{"--link mode on master", []upgrade.Option{upgrade.WithLinkMode()}},
			{"--link mode on segments", []upgrade.Option{upgrade.WithSegmentMode(), upgrade.WithLinkMode()}},
			{"--old-tablespaces-file flag on segments", []upgrade.Option{upgrade.WithTablespaceFile("tablespaceMappingFile.txt"), upgrade.WithSegmentMode()}},
			{"--old-options on master", []upgrade.Option{upgrade.WithOldOptions("option value")}},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				argsTest(t, c.options...)
			})
		}
	})
}
