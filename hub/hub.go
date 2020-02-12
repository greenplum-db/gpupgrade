package hub

import "github.com/greenplum-db/gpupgrade/utils"

//
// Build a hub-centric model of the world:
//
// A hub has agents, agents have segment pairs
//
func MakeHub(config *Config) Hub {
	var segmentPairsByHost = make(map[string][]SegmentPair)

	groupPrimaries(config, segmentPairsByHost)
	groupMirrors(config, segmentPairsByHost)

	return Hub{
		masterPair: SegmentPair{
			source: config.Source.Primaries[-1],
			target: config.Target.Primaries[-1],
		},
		agents: makeAgents(segmentPairsByHost),
	}
}

func makeAgents(segmentPairsByHost map[string][]SegmentPair) []Agent {
	var configs []Agent
	for hostname, agentSegmentPairs := range segmentPairsByHost {
		configs = append(configs, Agent{
			hostname:     hostname,
			segmentPairs: agentSegmentPairs,
		})
	}
	return configs
}

func groupMirrors(config *Config, pairs map[string][]SegmentPair) {
	sourceMap := config.Source.Mirrors
	targetMap := config.Target.Mirrors

	for contentId, sourceSegment := range sourceMap {
		if isStandby := contentId == 1; isStandby {
			continue
		}

		hostname := sourceSegment.Hostname

		if pairs[hostname] == nil {
			pairs[hostname] = []SegmentPair{}
		}

		segmentPair := SegmentPair{
			source: sourceSegment,
			target: targetMap[contentId],
		}

		pairs[hostname] = append(pairs[hostname], segmentPair)
	}
}

func groupPrimaries(config *Config, pairs map[string][]SegmentPair) {
	sourceMap := config.Source.Primaries
	targetMap := config.Target.Primaries

	for contentId, sourceSegment := range sourceMap {
		if isMaster := contentId == -1; isMaster {
			continue
		}

		hostname := sourceSegment.Hostname

		if pairs[hostname] == nil {
			pairs[hostname] = []SegmentPair{}
		}

		segmentPair := SegmentPair{
			source: sourceSegment,
			target: targetMap[contentId],
		}

		pairs[hostname] = append(pairs[hostname], segmentPair)
	}
}

type Hub struct {
	masterPair SegmentPair
	agents     []Agent
}

type Agent struct {
	hostname     string
	segmentPairs []SegmentPair
}

type SegmentPair struct {
	source utils.SegConfig
	target utils.SegConfig
}
