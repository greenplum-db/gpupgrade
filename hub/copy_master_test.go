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

	"github.com/greenplum-db/gpupgrade/utils"

	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
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

func TestCopyMaster(t *testing.T) {
	sourceCluster := MustCreateCluster(t, []utils.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
	})
	sourceCluster.BinDir = "/source/bindir"

	targetCluster := MustCreateCluster(t, []utils.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
	})
	targetCluster.BinDir = "/target/bindir"

	conf := &Config{
		Source:      sourceCluster,
		Target:      targetCluster,
		UseLinkMode: false,
	}
	hub := New(conf, grpc.DialContext, ".gpupgrade")

	t.Run("copies the master data directory to each primary host", func(t *testing.T) {
		// The verifier function can be called in parallel, so use a channel to
		// communicate which hosts were actually used.
		hosts := make(chan string, len(targetCluster.PrimaryHostnames()))

		// Validate the rsync call and arguments.
		execCommand = exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "rsync"
			if name != expected {
				t.Errorf("CopyMasterDataDir() invoked %q, want %q", name, expected)
			}

			// The last argument is host:/destination/directory. Remove the
			// host (saving it for later verification) to make comparison
			// easier.
			parts := strings.SplitN(args[len(args)-1], ":", 2)
			host, dest := parts[0], parts[1]
			args[len(args)-1] = dest

			expectedArgs := []string{
				"--archive", "--compress", "--delete", "--stats",
				"/data/qddir/seg-1/", "foobar/path",
			}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("rsync invoked with %q, want %q", args, expectedArgs)
			}

			hosts <- host
		})

		err := hub.CopyMasterDataDir(DevNull, "foobar/path")
		if err != nil {
			t.Errorf("copying master data directory: %+v", err)
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

	t.Run("copies the master data directory only once per host", func(t *testing.T) {
		// Create a one-host cluster.
		oneHostTargetCluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
			{ContentID: 0, DbID: 2, Port: 25432, Hostname: "localhost", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
			{ContentID: 1, DbID: 3, Port: 25433, Hostname: "localhost", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
		})
		oneHostTargetCluster.BinDir = "/target/bindir"

		hub.Target = oneHostTargetCluster
		defer func() { hub.Target = targetCluster }()

		// Validate the rsync call and arguments.
		execCommand = exectest.NewCommandWithVerifier(Success, func(name string, args ...string) {
			expected := "rsync"
			if name != expected {
				t.Errorf("CopyMasterDataDir() invoked %q, want %q", name, expected)
			}

			expectedArgs := []string{
				"--archive", "--compress", "--delete", "--stats",
				"/data/qddir/seg-1/", "localhost:foobar/path",
			}
			if !reflect.DeepEqual(args, expectedArgs) {
				t.Errorf("rsync invoked with %q, want %q", args, expectedArgs)
			}
		})

		err := hub.CopyMasterDataDir(DevNull, "foobar/path")
		if err != nil {
			t.Errorf("copying master data directory: %+v", err)
		}
	})

	t.Run("serializes rsync failures to the log stream", func(t *testing.T) {
		execCommand = exectest.NewCommand(RsyncFailure)
		buffer := new(bufferedStreams)

		err := hub.CopyMasterDataDir(buffer, "foobar/path")

		// Make sure the errors are correctly propagated up.
		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}
		var exitErr *exec.ExitError
		for _, err := range merr.Errors {
			if !xerrors.As(err, &exitErr) || exitErr.ExitCode() != rsyncExitCode {
				t.Errorf("returned error %#v, want exit code %d", err, rsyncExitCode)
			}
		}

		stdout := buffer.stdout.String()
		if len(stdout) != 0 {
			t.Errorf("got stdout %q, expected no output", stdout)
		}

		// Make sure we have as many copies of the stderr string as there are
		// hosts. They should be serialized sanely, even though we may execute
		// in parallel.
		stderr := buffer.stderr.String()
		expected := strings.Repeat(rsyncErrorMessage, len(targetCluster.PrimaryHostnames()))
		if stderr != expected {
			t.Errorf("got stderr:\n%v\nwant:\n%v", stderr, expected)
		}
	})

	t.Run("returns errors when writing stdout and stderr buffers to the stream", func(t *testing.T) {
		execCommand = exectest.NewCommand(StreamingMain)
		streams := failingStreams{errors.New("e")}

		err := hub.CopyMasterDataDir(streams, "")

		// Make sure the errors are correctly propagated up.
		var merr *multierror.Error
		if !xerrors.As(err, &merr) {
			t.Fatalf("returned %#v, want error type %T", err, merr)
		}
		for _, err := range merr.Errors {
			if !xerrors.Is(err, streams.err) {
				t.Errorf("returned error %#v, want %#v", err, streams.err)
			}
		}
	})
}
