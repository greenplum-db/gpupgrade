package commanders

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/utils"
)

// introduce this variable to allow exec.Command to be mocked out in tests
var execCommandHubStart = exec.Command
var execCommandHubCount = exec.Command

// we create the state directory in the cli to ensure that at most one gpupgrade is occuring
// at the same time.
func CreateStateDirAndClusterConfigs(sourceBinDir, targetBinDir string) (err error) {
	s := Substep("Creating directories...")
	defer s.Finish(&err)

	stateDir := utils.GetStateDir()
	err = os.Mkdir(stateDir, 0700)
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

func StartHub() (err error) {
	s := Substep("Starting hub...")
	defer s.Finish(&err)

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

func IsHubRunning() (bool, error) {
	script := `ps -ef | grep -wGc "[g]pupgrade_hub"` // use square brackets to avoid finding yourself in matches
	_, err := execCommandHubCount("bash", "-c", script).Output()

	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ProcessState.ExitCode() == 1 { // hub not found
			return false, nil
		}
	}
	if err != nil { // grep failed
		return false, err
	}

	return true, nil
}
