package hub

import (
	"fmt"
	"strconv"

	"github.com/greenplum-db/gpupgrade/utils"

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
func UpgradeStandby(r GreenplumRunner, standbyConfig StandbyConfig) (utils.SegConfig, error) {
	gplog.Info(fmt.Sprintf("removing any existing standby master"))

	err := r.Run("gpinitstandby", "-r", "-a")

	if err != nil {
		gplog.Debug(fmt.Sprintf(
			"error message from removing existing standby master (expected in the happy path): %v",
			err))
	}

	gplog.Info(fmt.Sprintf("creating new standby master: %#v", standbyConfig))

	err = r.Run("gpinitstandby",
		"-P", strconv.Itoa(standbyConfig.Port),
		"-s", standbyConfig.Hostname,
		"-S", standbyConfig.DataDirectory,
		"-a")

	return makeStandbySegConfig(standbyConfig), err
}

func makeStandbySegConfig(standbyConfig StandbyConfig) utils.SegConfig {
	return utils.SegConfig{
		DbID:      utils.NotSetDbID,
		ContentID: -1,
		Port:      standbyConfig.Port,
		Hostname:  standbyConfig.Hostname,
		DataDir:   standbyConfig.DataDirectory,
		Role:      utils.MirrorRole,
	}
}
