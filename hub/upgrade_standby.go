package hub

import (
	"fmt"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type UpgradeStandbyFunc func(config StandbyConfig) error

var upgradeStandbyFunc = upgradeStandby

type StandbyConfig struct {
	GreenplumRunner
	Port          int
	Hostname      string
	DataDirectory string
}

func UpgradeStandby(standbyConfig StandbyConfig) error {
	return upgradeStandbyFunc(standbyConfig)
}

func upgradeStandby(standbyConfig StandbyConfig) error {
	gplog.Info(fmt.Sprintf("removing any existing standby master"))

	err := gpinitstandbyRemove(standbyConfig)

	if err != nil {
		gplog.Debug(fmt.Sprintf(
			"error message from removing existing standby master (expected in the happy path): %v",
			err))
	}

	gplog.Info(fmt.Sprintf("creating new standby master: %#v", standbyConfig))

	err = gpinitstandbyCreate(standbyConfig)

	if err != nil {
		gplog.Error(fmt.Sprintf(
			"error message from creating new standby master: %v",
			err))
	}

	return err
}

//
// To ensure idempotency, remove any possible existing standby from the cluster
// before adding a new one.
//
// In the happy-path, we expect this to fail as there should not be an existing
// standby for the cluster.
//
func gpinitstandbyRemove(standbyConfig StandbyConfig) error {
	return standbyConfig.Run("gpinitstandby", "-r")
}

func gpinitstandbyCreate(standbyConfig StandbyConfig) error {
	return standbyConfig.Run("gpinitstandby",
		"-P", strconv.Itoa(standbyConfig.Port),
		"-s", standbyConfig.Hostname,
		"-S", standbyConfig.DataDirectory,
		"-a")
}
