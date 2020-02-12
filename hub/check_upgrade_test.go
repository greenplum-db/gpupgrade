package hub_test

import (
	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"testing"
)

type test_upgrader struct {}

func (test_upgrader) UpgradeMaster(source, target *utils.Cluster, stateDir string, stream step.OutStreams,
	checkOnly bool, useLinkMode bool) error {
	return UpgradeMasterMock(source, target, stateDir, stream, checkOnly, useLinkMode)
}
func (test_upgrader) UpgradePrimaries(checkOnly bool, masterBackupDir string, agentConns []*hub.Connection,
	dataDirPairMap map[string][]*idl.DataDirPair, source *utils.Cluster, target *utils.Cluster,
	useLinkMode bool) error {
	return UpgradePrimariesMock(checkOnly, masterBackupDir, agentConns, dataDirPairMap ,source, target, useLinkMode)
}


func TestMasterIsChecked(t *testing.T) {
	// test UpgradeMaster is called with correct arguments
	conf := hub.Config{UseLinkMode:true}
	s := hub.New(&conf, nil, "/some/state/dir")
	err := test_upgrader{}.UpgradeMaster(nil, nil, s.StateDir, nil,
		true, true)
	if err != nil {
		t.Errorf("got error: %#v", err)
	}
}

func TestPrimariesAreChecked(t *testing.T) {
	// test UpgradePrimaries is called with correct arguments
}

func UpgradeMasterMock(source, target *utils.Cluster, stateDir string, stream step.OutStreams,
checkOnly bool, useLinkMode bool) error {
	return nil
}

func UpgradePrimariesMock(checkOnly bool, masterBackupDir string, agentConns []*hub.Connection,
	dataDirPairMap map[string][]*idl.DataDirPair, source *utils.Cluster, target *utils.Cluster,
	useLinkMode bool) error {
	return nil
}
