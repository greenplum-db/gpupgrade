package greenplum

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/spyrunner"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}

func PgrepCmd() {}
func PgrepCmd_Errors() {
	os.Stderr.WriteString("exit status 2")
	os.Exit(2)
}

func init() {
	exectest.RegisterMains(
		PgrepCmd,
		PgrepCmd_Errors,
	)
}

// TODO: Consolidate with the same function in common_test.go in the
//  hub package. This is tricky due to cycle imports and other issues.
// MustCreateCluster creates a utils.Cluster and calls t.Fatalf() if there is
// any error.
func MustCreateCluster(t *testing.T, segs []SegConfig) *Cluster {
	t.Helper()

	cluster, err := NewCluster(segs)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return cluster
}

// TODO: Consolidate with the same function in common_test.go in the hub package.
// DevNull implements OutStreams by just discarding all writes.
var DevNull = devNull{}

type devNull struct{}

func (_ devNull) Stdout() io.Writer {
	return ioutil.Discard
}

func (_ devNull) Stderr() io.Writer {
	return ioutil.Discard
}

func TestStartOrStopCluster(t *testing.T) {
	g := NewGomegaWithT(t)

	source := MustCreateCluster(t, []SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "basedir/seg-1", Role: "p"},
	})
	source.BinDir = "/source/bindir"

	utils.System.RemoveAll = func(s string) error { return nil }
	utils.System.MkdirAll = func(s string, perm os.FileMode) error { return nil }

	pgrepCmd = nil

	defer func() {
		pgrepCmd = exec.Command
	}()

	t.Run("isPostmasterRunning succeeds", func(t *testing.T) {
		pgrepCmd = exectest.NewCommandWithVerifier(PgrepCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		command := pgrepCommand{streams: DevNull}
		err := command.isRunning(source.MasterPidFile())
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("isPostmasterRunning fails", func(t *testing.T) {
		pgrepCmd = exectest.NewCommand(PgrepCmd_Errors)

		command := pgrepCommand{streams: DevNull}
		err := command.isRunning(source.MasterPidFile())
		g.Expect(err).To(HaveOccurred())
	})

	pgrepCommand := &pgrepCommand{streams: DevNull}

	t.Run("stop cluster successfully shuts down cluster", func(t *testing.T) {
		pgrepCmd = exectest.NewCommandWithVerifier(PgrepCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		runner := spyrunner.New()

		gpUtilities := newGpStop(source, runner, pgrepCommand)
		err := gpUtilities.Stop()

		if err != nil {
			t.Fatalf("unexpected error while running gpstop: %v", err)
		}

		gpstopCall := runner.Call("gpstop", 1)

		if gpstopCall == nil {
			t.Fatalf("got no calls to gpstop, expected one call")
		}

		for _, arg := range []string{"-a", "-d", "basedir/seg-1"} {
			if !gpstopCall.ArgumentsInclude(arg) {
				t.Errorf("got no argument %v to gpstop, expected %v", arg, arg)
			}
		}
	})

	t.Run("stop cluster detects that cluster is already shutdown", func(t *testing.T) {
		pgrepCmd = exectest.NewCommand(PgrepCmd_Errors)

		var skippedStopClusterCommand = true
		startStopCmd = exectest.NewCommandWithVerifier(PgrepCmd,
			func(path string, args ...string) {
				skippedStopClusterCommand = false
			})

		gpUtilities := newGpStop(source, spyrunner.New(), pgrepCommand)
		err := gpUtilities.Stop()

		g.Expect(err).To(HaveOccurred())
		g.Expect(skippedStopClusterCommand).To(Equal(true))
	})

	t.Run("start cluster successfully starts up cluster", func(t *testing.T) {
		runner := spyrunner.New()

		gpStart := newGpStart(source, runner)
		err := gpStart.Start()

		if err != nil {
			t.Fatalf("unexpected error while running gpstart: %v", err)
		}

		gpstartCall := runner.Call("gpstart", 1)

		if gpstartCall == nil {
			t.Fatalf("got no calls to gpstop, expected one call")
		}

		for _, arg := range []string{"-a", "-d", "basedir/seg-1"} {
			if !gpstartCall.ArgumentsInclude(arg) {
				t.Errorf("got no argument %v to gpstop, expected %v", arg, arg)
			}
		}
	})

	t.Run("start master successfully starts up master only", func(t *testing.T) {
		runner := spyrunner.New()

		gpStart := newGpStart(source, runner)

		err := gpStart.StartMasterOnly()
		if err != nil {
			t.Fatalf("unexpected error while running gpstart: %v", err)
		}

		gpstartCall := runner.Call("gpstart", 1)

		if gpstartCall == nil {
			t.Fatalf("got no calls to gpstop, expected one call")
		}

		for _, arg := range []string{"-m", "-a", "-d", "basedir/seg-1"} {
			if !gpstartCall.ArgumentsInclude(arg) {
				t.Errorf("got no argument %v to gpstop, expected %v", arg, arg)
			}
		}
	})

	t.Run("stop master successfully shuts down master only", func(t *testing.T) {
		pgrepCmd = exectest.NewCommandWithVerifier(PgrepCmd,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "pgrep -F basedir/seg-1/postmaster.pid"}))
			})

		runner := spyrunner.New()

		gpStop := newGpStop(source, runner, pgrepCommand)
		err := gpStop.StopMasterOnly()

		if err != nil {
			t.Fatalf("unexpected error while running gpstop: %v", err)
		}

		gpstopCall := runner.Call("gpstop", 1)

		if gpstopCall == nil {
			t.Fatalf("got no calls to gpstop, expected one call")
		}

		for _, arg := range []string{"-m", "-a", "-d", "basedir/seg-1"} {
			if !gpstopCall.ArgumentsInclude(arg) {
				t.Errorf("got no argument %v to gpstop, expected %v", arg, arg)
			}
		}
	})
}
