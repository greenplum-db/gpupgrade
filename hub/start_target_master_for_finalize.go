package hub

import "github.com/greenplum-db/gpupgrade/step"

func StartTargetMasterForFinalize(streams step.OutStreams, config *Config) error {
	// We need to pass a modified Target cluster to StartMasterOnly because
	// the data directory has been promoted to its new location
	var target = *config.Target
	targetMaster := target.Primaries[-1]
	targetMaster.DataDir = targetMaster.PublishingDataDirectory(config.Source.Primaries[-1])
	target.Primaries[-1] = targetMaster

	return StartMasterOnly(streams, &target, false)
}
