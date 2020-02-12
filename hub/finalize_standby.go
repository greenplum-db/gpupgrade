package hub

import "github.com/greenplum-db/gpupgrade/step"

func FinalizeStandby(c *Config, streams step.OutStreams) error {
	greenplumRunner := &greenplumRunner{
		masterPort:          c.Target.MasterPort(),
		masterDataDirectory: c.Target.MasterDataDir(),
		binDir:              c.Target.BinDir,
		streams:             streams,
	}

	standbyConfig := StandbyConfig{
		Port:          c.TargetPorts.Standby,
		Hostname:      c.Source.StandbyHostname(),
		DataDirectory: c.Source.StandbyDataDirectory() + "_upgrade",
	}

	newConfig, standbyError := UpgradeStandby(greenplumRunner, standbyConfig)

	if standbyError == nil {
		c.Target.Mirrors[-1] = newConfig
	}

	return standbyError
}
