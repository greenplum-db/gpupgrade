package hub

import (
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/step"
)

type UpgradeChecker interface {
	UpgradeMaster(args UpgradeMasterArgs) error
	UpgradePrimaries(args UpgradePrimaryArgs) error
}

type upgradeChecker struct{}

func (upgradeChecker) UpgradeMaster(args UpgradeMasterArgs) error {
	return UpgradeMaster(args)
}

func (upgradeChecker) UpgradePrimaries(args UpgradePrimaryArgs) error {
	return UpgradePrimaries(args)
}

type AgentConnProvider interface {
	GetAgents(s *Server) ([]*Connection, error)
}

type agentConnProvider struct{}

func (agentConnProvider) GetAgents(s *Server) ([]*Connection, error) {
	return s.AgentConns()
}

var upgrader UpgradeChecker = upgradeChecker{}
var agentProvider AgentConnProvider = agentConnProvider{}

func (s *Server) CheckUpgrade(stream step.OutStreams) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		checkErrs <- upgrader.UpgradeMaster(UpgradeMasterArgs{
			Source:      s.Source,
			Target:      s.Target,
			StateDir:    s.StateDir,
			Stream:      stream,
			CheckOnly:   true,
			UseLinkMode: s.UseLinkMode,
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		conns, connsErr := agentProvider.GetAgents(s)
		if connsErr != nil {
			checkErrs <- errors.Wrap(connsErr, "failed to connect to gpupgrade agents")
			return
		}

		dataDirPairMap, dataDirPairsErr := s.GetDataDirPairs()
		if dataDirPairsErr != nil {
			checkErrs <- errors.Wrap(dataDirPairsErr, "failed to get old and new primary data directories")
			return
		}

		checkErrs <- upgrader.UpgradePrimaries(UpgradePrimaryArgs{
			CheckOnly:       true,
			MasterBackupDir: "",
			AgentConns:      conns,
			DataDirPairMap:  dataDirPairMap,
			Source:          s.Source,
			Target:          s.Target,
			UseLinkMode:     s.UseLinkMode,
		})
	}()

	wg.Wait()
	close(checkErrs)

	var multiErr *multierror.Error
	for err := range checkErrs {
		multiErr = multierror.Append(multiErr, err)
	}

	return multiErr.ErrorOrNil()
}
