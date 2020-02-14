package hub

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

type GreenplumRunner interface {
	ShellRunner
	BinDir() string
	MasterDataDirectory() string
	MasterPort() int
}

func (e *greenplumRunner) Run(utilityName string, arguments ...string) error {
	commandAsString := exec.Command(
		filepath.Join(e.binDir, utilityName), arguments...,
	).String()

	withGreenplumPath := fmt.Sprintf("source %s/../greenplum_path.sh && %s", e.binDir, commandAsString)

	command := exec.Command("bash", "-c", withGreenplumPath)
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "MASTER_DATA_DIRECTORY", e.masterDataDirectory))
	command.Env = append(command.Env, fmt.Sprintf("%v=%v", "PGPORT", e.masterPort))
	output, err := command.CombinedOutput()

	fmt.Printf("Master data directory, %v\n", e.masterDataDirectory)
	fmt.Printf("%s: %s \n", command.String(), string(output))

	return err
}

type greenplumRunner struct {
	binDir              string
	masterDataDirectory string
	masterPort          int
}

func (e *greenplumRunner) BinDir() string {
	return e.binDir
}

func (e *greenplumRunner) MasterDataDirectory() string {
	return e.masterDataDirectory
}

func (e *greenplumRunner) MasterPort() int {
	return e.masterPort
}
