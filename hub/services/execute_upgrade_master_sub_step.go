package services

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Allow exec.Command to be mocked out by exectest.NewCommand.
var execCommand = exec.Command

func (h *Hub) ExecuteUpgradeMasterSubStep(stream idl.CliToHub_ExecuteServer) error {
	gplog.Info("starting %s", upgradestatus.CONVERT_MASTER)

	step, err := h.InitializeStep(upgradestatus.CONVERT_MASTER)
	if err != nil {
		gplog.Error(err.Error())
		return err
	}

	err = h.UpgradeMaster(stream)
	if err != nil {
		gplog.Error(err.Error())
		step.MarkFailed()
	} else {
		step.MarkComplete()
	}

	return err
}

func (h *Hub) UpgradeMaster(stream idl.CliToHub_ExecuteServer) error {
	// Make sure our working directory exists.
	wd := utils.MasterPGUpgradeDirectory(h.conf.StateDir)
	err := utils.System.MkdirAll(wd, 0700)
	if err != nil {
		return err
	}

	// Create a log file to contain pg_upgrade output.
	log, err := utils.System.OpenFile(
		filepath.Join(wd, "pg_upgrade.log"),
		os.O_WRONLY|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}

	pair := clusterPair{h.source, h.target}
	return pair.ConvertMaster(stream, log, wd)
}

// clusterPair simply holds the source and target clusters.
type clusterPair struct {
	Source, Target *utils.Cluster
}

// ConvertMaster invokes pg_upgrade on the local master data directory from the
// given working directory, which must exist prior to invocation. It streams all
// standard output and error from pg_upgrade to the given io.Writer (though the
// order in which those streams interleave is inherently nondeterministic), and
// additionally sends the data through the given gRPC stream.
//
// Errors when writing to the io.Writer are fatal, but errors encountered during
// gRPC streaming are logged and otherwise ignored. The pg_upgrade execution
// will continue even if the client disconnects.
func (c clusterPair) ConvertMaster(stream idl.CliToHub_ExecuteServer, out io.Writer, wd string) error {
	mux := newMultiplexedStream(stream, out)

	path := filepath.Join(c.Target.BinDir, "pg_upgrade")
	cmd := execCommand(path,
		"--old-bindir", c.Source.BinDir,
		"--old-datadir", c.Source.MasterDataDir(),
		"--old-port", strconv.Itoa(c.Source.MasterPort()),
		"--new-bindir", c.Target.BinDir,
		"--new-datadir", c.Target.MasterDataDir(),
		"--new-port", strconv.Itoa(c.Target.MasterPort()),
		"--mode=dispatcher",
	)

	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)
	cmd.Dir = wd

	// Explicitly clear the child environment. pg_upgrade shouldn't need things
	// like PATH, and PGPORT et al are explicitly forbidden to be set.
	cmd.Env = []string{}

	// XXX ...but we make a single exception for now, for LD_LIBRARY_PATH, to
	// work around pervasive problems with RPATH settings in our Postgres
	// extension modules.
	if path, ok := os.LookupEnv("LD_LIBRARY_PATH"); ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("LD_LIBRARY_PATH=%s", path))
	}

	return cmd.Run()
}
