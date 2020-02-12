package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func SwapDataDirectories(hub Hub, agentBroker AgentBroker) error {
	sourceSegment := hub.masterPair.source
	targetSegment := hub.masterPair.target

	if err := archive(sourceSegment); err != nil {
		return err
	}

	if err := publish(targetSegment, sourceSegment); err != nil {
		return err
	}

	if err := swapOnAgents(hub.agents, agentBroker); err != nil {
		return err
	}

	return nil
}

func archive(sourceSegment utils.SegConfig) error {
	return renameDirectory(sourceSegment.DataDir, sourceSegment.ArchivingDataDirectory())
}

func publish(targetSegment utils.SegConfig, sourceSegment utils.SegConfig) error {
	return renameDirectory(targetSegment.DataDir, targetSegment.PublishingDataDirectory(sourceSegment))
}

func swapOnAgents(agents []Agent, agentBroker AgentBroker) error {
	err := &multierror.Error{}

	result := make(chan error, len(agents))

	for _, agent := range agents {
		agent := agent // capture agent variable

		go func() {
			result <- agentBroker.ReconfigureDataDirectories(agent.hostname,
				makeRenamePairs(agent.segmentPairs))
		}()
	}

	for range agents {
		newError := <-result
		multierror.Append(err, newError)
	}

	return err.ErrorOrNil()
}

func makeRenamePairs(pairs []SegmentPair) []*idl.RenamePair {
	var renamePairs []*idl.RenamePair

	for _, pair := range pairs {
		archivePair := makeArchivePair(pair)
		publishPair := makePublishPair(pair)

		if isFull(archivePair) && isFull(publishPair) {
			renamePairs = append(renamePairs, &archivePair, &publishPair)
		}
	}

	return renamePairs
}

func makePublishPair(pair SegmentPair) idl.RenamePair {
	return idl.RenamePair{
		Src: pair.target.DataDir,
		Dst: pair.target.PublishingDataDirectory(pair.source),
	}
}

func makeArchivePair(pair SegmentPair) idl.RenamePair {
	return idl.RenamePair{
		Src: pair.source.DataDir,
		Dst: pair.source.ArchivingDataDirectory(),
	}
}

func isFull(pair idl.RenamePair) bool {
	return pair.Dst != "" && pair.Src != ""
}

func renameDirectory(originalName, newName string) error {
	gplog.Info(fmt.Sprintf("moving directory %v to %v", originalName, newName))

	return utils.System.Rename(originalName, newName)
}
