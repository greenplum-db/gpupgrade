// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/rsync"
)

const (
	rsyncExitCode     int    = 23 // rsync returns 23 for a partial transfer
	rsyncErrorMessage string = `rsync: recv_generator: mkdir "/tmp/master_copy/gpseg-1" failed: Permission denied(13)
*** Skipping any contents from this failed directory ***
rsync error: some files/attrs were not transferred (see previous errors) (code 23) atmain.c(1052) [sender=3.0.9]
`
)

func RsyncFailure() {
	fmt.Fprint(os.Stderr, rsyncErrorMessage)
	os.Exit(rsyncExitCode)
}

func init() {
	exectest.RegisterMains(
		RsyncFailure,
	)
}

func TestCopy(t *testing.T) {
	testlog.SetupLogger()

	t.Run("copies the directory only once per host", func(t *testing.T) {
		sourceDir := []string{"/data/qddir/seg-1/"}
		targetHosts := []string{"localhost"}

		// Validate the rsync call and arguments.
		cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "rsync"
			if name != expected {
				t.Errorf("Copy() invoked %q, want %q", name, expected)
			}

			expectedArgs := []string{
				"--archive", "--compress", "--delete", "--stats",
				"/data/qddir/seg-1/", "localhost:foobar/path",
			}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("rsync invoked with %q, want %q", args, expectedArgs)
			}
		})
		rsync.SetRsyncCommand(cmd)

		err := Copy(step.DevNullStream, "foobar/path", sourceDir, targetHosts)
		if err != nil {
			t.Errorf("copying data directory: %+v", err)
		}
	})

	t.Run("copies the data directory to each host", func(t *testing.T) {
		// The verifier function can be called in parallel, so use a channel to
		// communicate which hosts were actually used.
		primaryHosts := []string{"host1", "host2"}
		hosts := make(chan string, len(primaryHosts))
		sourceDir := []string{"/data/qddir/seg-1"}

		expectedArgs := []string{
			"--archive", "--compress", "--delete", "--stats",
			"/data/qddir/seg-1", "foobar/path",
		}
		execCommandVerifier(t, hosts, expectedArgs)

		err := Copy(step.DevNullStream, "foobar/path", sourceDir, primaryHosts)
		if err != nil {
			t.Errorf("copying directory: %+v", err)
		}

		close(hosts)

		// Collect the hostnames for validation.
		var actualHosts []string
		for host := range hosts {
			actualHosts = append(actualHosts, host)
		}
		sort.Strings(actualHosts) // receive order not guaranteed

		expectedHosts := []string{"host1", "host2"}
		if !reflect.DeepEqual(actualHosts, expectedHosts) {
			t.Errorf("copied to hosts %q, want %q", actualHosts, expectedHosts)
		}
	})

	t.Run("returns errors when writing stdout and stderr buffers to the stream", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(StreamingMain))
		streams := testutils.FailingStreams{Err: errors.New("e")}

		err := Copy(streams, "", nil, []string{"localhost"})

		// Make sure the errors are correctly propagated up.
		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("returned %#v, want error type %T", err, errs)
		}
		for _, err := range errs {
			if !errors.Is(err, streams.Err) {
				t.Errorf("returned error %#v, want %#v", err, streams.Err)
			}
		}
	})

	t.Run("serializes rsync failures to the log stream", func(t *testing.T) {
		rsync.SetRsyncCommand(exectest.NewCommand(RsyncFailure))
		buffer := new(step.BufferedStreams)
		hosts := []string{"mdw", "sdw1", "sdw2"}

		err := Copy(buffer, "foobar/path", nil, hosts)

		// Make sure the errors are correctly propagated up.
		var errs errorlist.Errors
		if !errors.As(err, &errs) {
			t.Fatalf("returned %#v, want error type %T", err, errs)
		}

		var exitErr *exec.ExitError
		for _, err := range errs {
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != rsyncExitCode {
				t.Errorf("returned error %#v, want exit code %d", err, rsyncExitCode)
			}
		}

		stdout := buffer.StdoutBuf.String()
		if len(stdout) != 0 {
			t.Errorf("got stdout %q, expected no output", stdout)
		}

		// Make sure we have as many copies of the stderr string as there are
		// hosts. They should be serialized sanely, even though we may execute
		// in parallel.
		stderr := buffer.StderrBuf.String()
		expected := strings.Repeat(rsyncErrorMessage, len(hosts))
		if stderr != expected {
			t.Errorf("got stderr:\n%v\nwant:\n%v", stderr, expected)
		}
	})
}

func TestCopyMasterDataDir(t *testing.T) {
	testhelper.SetupTestLogger()

	intermediateTarget := MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	t.Run("copies the master data directory to each primary host", func(t *testing.T) {
		// The verifier function can be called in parallel, so use a channel to
		// communicate which hosts were actually used.
		hosts := make(chan string, len(intermediateTarget.PrimaryHostnames()))

		expectedArgs := []string{
			"--archive", "--compress", "--delete", "--stats",
			"/data/qddir/seg-1/", "foobar/path",
		}

		execCommandVerifier(t, hosts, expectedArgs)

		err := CopyMasterDataDir(step.DevNullStream, intermediateTarget.MasterDataDir(), "foobar/path", intermediateTarget.PrimaryHostnames())
		if err != nil {
			t.Errorf("copying master data directory: %+v", err)
		}

		close(hosts)

		expectedHosts := []string{"host1", "host2"}
		verifyHosts(hosts, expectedHosts, t)
	})
}

func TestCopyMasterTablespaces(t *testing.T) {
	testhelper.SetupTestLogger()

	intermediateTarget := MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})

	TablespacesMappingFilePath := "/tmp/mapping.txt"

	Tablespaces := greenplum.Tablespaces{
		1: greenplum.SegmentTablespaces{
			1663: greenplum.TablespaceInfo{
				Location:    "/tmp/tblspc1",
				UserDefined: 0},
			1664: greenplum.TablespaceInfo{
				Location:    "/tmp/tblspc2",
				UserDefined: 1},
		},
		2: greenplum.SegmentTablespaces{
			1663: greenplum.TablespaceInfo{
				Location:    "/tmp/primary1/tblspc1",
				UserDefined: 0},
			1664: greenplum.TablespaceInfo{
				Location:    "/tmp/primary1/tblspc2",
				UserDefined: 1},
		},
		3: greenplum.SegmentTablespaces{
			1663: greenplum.TablespaceInfo{
				Location:    "/tmp/primary2/tblspc1",
				UserDefined: 0},
			1664: greenplum.TablespaceInfo{
				Location:    "/tmp/primary2/tblspc2",
				UserDefined: 1},
		},
	}

	t.Run("copies tablespace mapping file and master tablespace directory to each primary host", func(t *testing.T) {
		// The verifier function can be called in parallel, so use a channel to
		// communicate which hosts were actually used.
		hosts := make(chan string, len(intermediateTarget.PrimaryHostnames()))

		expectedArgs := []string{
			"--archive", "--compress", "--delete", "--stats",
			"/tmp/mapping.txt", "/tmp/tblspc2", "foobar/path/",
		}
		execCommandVerifier(t, hosts, expectedArgs)

		err := CopyMasterTablespaces(step.DevNullStream, TablespacesMappingFilePath, Tablespaces, "foobar/path", intermediateTarget.PrimaryHostnames())
		if err != nil {
			t.Errorf("copying master tablespace directories and mapping file: %+v", err)
		}

		close(hosts)

		expectedHosts := []string{"host1", "host2"}
		verifyHosts(hosts, expectedHosts, t)
	})

	t.Run("CopyMasterTablespaces returns nil if there is no tablespaces", func(t *testing.T) {
		// The verifier function can be called in parallel, so use a channel to
		// communicate which hosts were actually used.
		hosts := make(chan string, len(intermediateTarget.PrimaryHostnames()))

		var expectedArgs []string
		execCommandVerifier(t, hosts, expectedArgs)

		err := CopyMasterTablespaces(step.DevNullStream, TablespacesMappingFilePath, nil, "foobar/path", intermediateTarget.PrimaryHostnames())
		if err != nil {
			t.Errorf("got %+v, want nil", err)
		}

		close(hosts)

		if expectedArgs != nil {
			t.Errorf("Rsync() should not be invoked")
		}
	})
}

func verifyHosts(hosts chan string, expectedHosts []string, t *testing.T) {
	// Collect the hostnames for validation.
	var actualHosts []string
	for host := range hosts {
		actualHosts = append(actualHosts, host)
	}
	sort.Strings(actualHosts) // receive order not guaranteed

	if !reflect.DeepEqual(actualHosts, expectedHosts) {
		t.Errorf("copied to hosts %q, want %q", actualHosts, expectedHosts)
	}
}

// Validate the rsync call and arguments.
func execCommandVerifier(t *testing.T, hosts chan string, expectedArgs []string) {
	cmd := exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
		expected := "rsync"
		if name != expected {
			t.Errorf("invoked %q, want %q", name, expected)
		}

		// The last argument is host:/destination/directory. Remove the
		// host (saving it for later verification) to make comparison
		// easier.
		parts := strings.SplitN(args[len(args)-1], ":", 2)
		host, dest := parts[0], parts[1]
		args[len(args)-1] = dest

		if !reflect.DeepEqual(args, expectedArgs) {
			t.Errorf("rsync invoked with %q, want %q", args, expectedArgs)
		}

		hosts <- host
	})
	rsync.SetRsyncCommand(cmd)
}
