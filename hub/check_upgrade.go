package hub

import (
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"sync"

	"github.com/pkg/errors"

	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/step"
)

//func UpgradeMaster(source, target *utils.Cluster, stateDir string, stream step.OutStreams,
//   checkOnly bool, useLinkMode bool) error {
//func UpgradePrimaries(checkOnly bool, masterBackupDir string, agentConns []*Connection,
//   dataDirPairMap map[string][]*idl.DataDirPair, source *utils.Cluster, target *utils.Cluster, useLinkMode bool) error {

type upgrade_checker interface {
	UpgradeMaster(source, target *utils.Cluster, stateDir string, stream step.OutStreams,
	  checkOnly bool, useLinkMode bool) error
	UpgradePrimaries(checkOnly bool, masterBackupDir string, agentConns []*Connection,
	  dataDirPairMap map[string][]*idl.DataDirPair, source *utils.Cluster, target *utils.Cluster,
	  useLinkMode bool) error
}

type upgrader struct {}

func (upgrader) UpgradeMaster(source, target *utils.Cluster, stateDir string, stream step.OutStreams,
	checkOnly bool, useLinkMode bool) error {
	return UpgradeMaster(source, target, stateDir, stream, checkOnly, useLinkMode)
}
func (upgrader) UpgradePrimaries(checkOnly bool, masterBackupDir string, agentConns []*Connection,
	dataDirPairMap map[string][]*idl.DataDirPair, source *utils.Cluster, target *utils.Cluster,
	useLinkMode bool) error {
	return UpgradePrimaries(checkOnly, masterBackupDir, agentConns, dataDirPairMap ,source, target, useLinkMode)
}

func (s *Server) CheckUpgrade(stream step.OutStreams) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		checkErrs <- upgrader{}.UpgradeMaster(s.Source, s.Target, s.StateDir, stream, true, s.UseLinkMode)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		agentConns, agentConnsErr := s.AgentConns()

		if agentConnsErr != nil {
			checkErrs <- errors.Wrap(agentConnsErr, "failed to connect to gpupgrade agent")
			return
		}

		dataDirPairMap, dataDirPairsErr := s.GetDataDirPairs()

		if dataDirPairsErr != nil {
			checkErrs <- errors.Wrap(dataDirPairsErr, "failed to get old and new primary data directories")
			return
		}

		checkErrs <- upgrader{}.UpgradePrimaries(true, "", agentConns, dataDirPairMap, s.Source, s.Target, s.UseLinkMode)
	}()

	wg.Wait()
	close(checkErrs)

	var multiErr *multierror.Error
	for err := range checkErrs {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	return multiErr.ErrorOrNil()
}
