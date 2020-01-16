package commanders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/utils"
)

// introduce this variable to allow exec.Command to be mocked out in tests
var execCommandHubStart = exec.Command
var execCommandHubCount = exec.Command

// we create the state directory in the cli to ensure that at most one gpupgrade is occurring
// at the same time.
func CreateStateDir() (err error) {
	s := Substep("Creating state directory...")
	defer s.Finish(&err)

	stateDir := utils.GetStateDir()
	err = os.Mkdir(stateDir, 0700)
	if os.IsExist(err) {
		gplog.Debug("State directory %s already present...skipping", stateDir)
		return nil
	}
	if err != nil {
		gplog.Debug("State directory %s could not be created.", stateDir)
		return err
	}

	return nil
}

func CreateInitialClusterConfigs() (err error) {
	s := Substep("Creating initial cluster config files...")
	defer s.Finish(&err)

	// if empty json configuration file exists, skip recreating it
	filename := filepath.Join(utils.GetStateDir(), hub.ConfigFileName)
	_, err = os.Stat(filename)

	// if the file exists, there will be no error or if there is an error it might
	// also indicate that the file exists, in either case don't overwrite the file
	if err == nil || os.IsExist(err) {
		gplog.Debug("Initial cluster configuration file %s already present...skipping", filename)
		return nil
	}

	// if the err is anything other than file does not exist, error out
	if !os.IsNotExist(err) {
		gplog.Debug("Check to find presence of initial cluster configuration file %s failed", filename)
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "{}") // the hub will fill this in during initialization
	return nil
}

func StartHub() (err error) {
	s := Substep("Starting hub...")
	defer s.Finish(&err)

	running, err := IsHubRunning()
	if err != nil {
		gplog.Error("failed to determine if hub already running")
		return err
	}
	if running {
		gplog.Debug("gpupgrade hub already running...skipping.")
		return nil
	}

	cmd := execCommandHubStart("gpupgrade", "hub", "--daemonize")
	stdout, cmdErr := cmd.Output()
	if cmdErr != nil {
		err := fmt.Errorf("failed to start hub (%s)", cmdErr)
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			// Annotate with the Stderr capture, if we have it.
			err = fmt.Errorf("%s: %s", err, exitErr.Stderr)
		}
		return err
	}
	gplog.Debug("gpupgrade hub started successfully: %s", stdout)
	return nil
}

func IsHubRunning() (bool, error) {
	script := `ps -ef | grep -wGc "[g]pupgrade hub"` // use square brackets to avoid finding yourself in matches
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
