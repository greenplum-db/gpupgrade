package commanders

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/cluster"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/idl"
)

// introduce this variable to allow exec.Command to be mocked out in tests
var execCommandHubStart = exec.Command
var execCommandHubCount = exec.Command

// we create the state directory in the cli to ensure that at most one gpupgrade is occuring
// at the same time.
func CreateStateDirAndClusterConfigs(sourceBinDir, targetBinDir string) error {
	stateDir := utils.GetStateDir()
	err := os.Mkdir(stateDir, 0700)
	if os.IsExist(err) {
		return fmt.Errorf("gpupgrade state dir (%s) already exists. Did you already run gpupgrade initialize?", stateDir)
	} else if err != nil {
		return err
	}

	// Create empty clusters in source and target so that gpupgrade_hub can
	// start without having replaced them with current values.
	// TODO: implement a slicker scheme to allow this.
	emptyCluster := cluster.NewCluster([]cluster.SegConfig{})

	source := &utils.Cluster{
		Cluster:    emptyCluster,
		BinDir:     path.Clean(sourceBinDir),
		ConfigPath: filepath.Join(stateDir, utils.SOURCE_CONFIG_FILENAME),
	}
	err = source.Commit()
	if err != nil {
		return errors.Wrap(err, "Unable to save empty source cluster configuration")
	}

	target := &utils.Cluster{
		Cluster:    emptyCluster,
		BinDir:     path.Clean(targetBinDir),
		ConfigPath: filepath.Join(stateDir, utils.TARGET_CONFIG_FILENAME),
	}
	err = target.Commit()
	if err != nil {
		return errors.Wrap(err, "Unable to save empty target cluster configuration")
	}

	return nil
}

// TODO: how should we find the gpupgrade_hub executable?  Right now, it's via newBinDir
func StartHub() error {
	countHubs, err := HowManyHubsRunning()
	if err != nil {
		gplog.Error("failed to determine if hub already running")
		return err
	}
	if countHubs >= 1 {
		gplog.Error("gpupgrade_hub process already running")
		return errors.New("gpupgrade_hub process already running")
	}

	cmd := execCommandHubStart("gpupgrade_hub", "--daemonize")
	stdout, cmdErr := cmd.Output()
	if cmdErr != nil {
		err := fmt.Errorf("failed to start hub (%s)", cmdErr)
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			// Annotate with the Stderr capture, if we have it.
			err = fmt.Errorf("%s: %s", err, exitErr.Stderr)
		}
		return err
	}
	gplog.Debug("gpupgrade_hub started successfully: %s", stdout)
	return nil
}

func Initialize(client idl.CliToHubClient, oldBinDir, newBinDir string, oldPort int) (err error) {
	request := &idl.InitializeRequest{
		OldBinDir: oldBinDir,
		NewBinDir: newBinDir,
		OldPort:   int32(oldPort),
	}
	_, err = client.Initialize(context.Background(), request)
	if err != nil {
		return errors.Wrap(err, "initializing hub")
	}

	return nil
}

func HowManyHubsRunning() (int, error) {
	howToLookForHub := `ps -ef | grep -Gc "[g]pupgrade_hub$"` // use square brackets to avoid finding yourself in matches
	output, err := execCommandHubCount("bash", "-c", howToLookForHub).Output()
	value, convErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if convErr != nil {
		if err != nil {
			return -1, err
		}
		return -1, convErr
	}

	// let value == 0 through before checking err, for when grep finds nothing and its error-code is 1
	if value >= 0 {
		return value, nil
	}

	// only needed if the command errors, but somehow put a parsable & negative value on stdout
	return -1, err
}
