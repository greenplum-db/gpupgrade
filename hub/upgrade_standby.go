package hub

import (
	"fmt"
	"strconv"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type StandbyConfig struct {
	Port          int
	Hostname      string
	DataDirectory string
}

//
// To ensure idempotency, remove any possible existing standby from the cluster
// before adding a new one.
//
// In the happy-path, we expect this to fail as there should not be an existing
// standby for the cluster.
//
func UpgradeStandby(r GreenplumRunner, standbyConfig StandbyConfig) error {
	gplog.Info(fmt.Sprintf("removing any existing standby master on target cluster"))

	err := r.Run("gpinitstandby", "-r", "-a")

	if err != nil {
		gplog.Debug(fmt.Sprintf(
			"error message from removing existing standby master (expected in the happy path): %v",
			err))
	}

	gplog.Info(fmt.Sprintf("creating target standby master: %#v", standbyConfig))

	return r.Run("gpinitstandby",
		"-P", strconv.Itoa(standbyConfig.Port),
		"-s", standbyConfig.Hostname,
		"-S", standbyConfig.DataDirectory,
		"-a")
}
