package hub

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/utils"
)

type upgraderMock struct {
	s *Server
}

func (u upgraderMock) UpgradeMaster(args UpgradeMasterArgs) error {
	return UpgradeMasterMock(args, u.s)
}

func (u upgraderMock) UpgradePrimaries(args UpgradePrimaryArgs) error {
	return UpgradePrimariesMock(args, u.s)
}

type agentConnSourceMock struct {
	conns []*Connection
}

func (a agentConnSourceMock) GetAgents(s *Server) ([]*Connection, error) {
	return a.conns, nil
}

var agentsSource = agentConnSourceMock{[]*Connection{&Connection{Conn: nil, Hostname: "bengie"}}}

func setUpgrader(updated UpgradeChecker) {
	upgrader = updated
}
func resetUpgrader() {
	upgrader = upgradeChecker{}
}

func setAgentProvider(agents AgentConnProvider) {
	agentProvider = agents
}
func resetAgentProvider() {
	agentProvider = agentConnProvider{}
}

func TestMasterIsCheckedLinkModeTrue(t *testing.T) {
	setAgentProvider(agentsSource)
	defer resetAgentProvider()

	sourceCluster := MustCreateCluster(t, []utils.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
	})
	targetCluster := MustCreateCluster(t, []utils.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p", PreferredRole: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p", PreferredRole: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p", PreferredRole: "p"},
	})
	var stateDirExpected = "/some/state/dir"

	for _, linkMode := range []bool{true, false} {
		t.Run(fmt.Sprintf("check upgrade correctly passes useLinkMode is %v", linkMode), func(t *testing.T) {
			conf := &Config{
				Source:      sourceCluster,
				Target:      targetCluster,
				UseLinkMode: linkMode,
			}
			s := New(conf, grpc.DialContext, stateDirExpected)
			testUpgraderMock := upgraderMock{s}

			setUpgrader(testUpgraderMock)
			defer resetUpgrader()

			err := s.CheckUpgrade(nil)
			if err != nil {
				t.Errorf("got error: %+v", err) // yes, '%+v'; '%#v' prints opaque multierror
			}
		})
	}
}

func UpgradeMasterMock(result UpgradeMasterArgs, expected *Server) error {
	if !reflect.DeepEqual(result.Source, expected.Source) {
		return errors.New(fmt.Sprintf("got %#v, expected %#v", result.Source, expected.Source))
	}
	if !reflect.DeepEqual(result.Target, expected.Target) {
		return errors.New(fmt.Sprintf("got %#v, expected %#v", result.Target, expected.Target))
	}
	if result.StateDir != expected.StateDir {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.StateDir, expected.StateDir))
	}
	// does not seem worth testing stream right now
	if result.CheckOnly != true {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.CheckOnly, true))
	}
	if result.UseLinkMode != expected.UseLinkMode {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.UseLinkMode, expected.UseLinkMode))
	}
	return nil
}

func UpgradePrimariesMock(result UpgradePrimaryArgs, expected *Server) error {
	if result.CheckOnly != true {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.CheckOnly, true))
	}
	if result.MasterBackupDir != "" {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.MasterBackupDir, ""))
	}
	if !reflect.DeepEqual(result.AgentConns, agentsSource.conns) {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.AgentConns, agentsSource.conns))
	}
	expectedDataDirs, _ := expected.GetDataDirPairs()
	if !reflect.DeepEqual(result.DataDirPairMap, expectedDataDirs) {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.DataDirPairMap, expectedDataDirs))
	}
	if !reflect.DeepEqual(result.Source, expected.Source) {
		return errors.New(fmt.Sprintf("got %#v, expected %#v", result.Source, expected.Source))
	}
	if !reflect.DeepEqual(result.Target, expected.Target) {
		return errors.New(fmt.Sprintf("got %#v, expected %#v", result.Target, expected.Target))
	}
	if result.UseLinkMode != expected.UseLinkMode {
		return errors.New(fmt.Sprintf("got %#v expected %#v", result.UseLinkMode, expected.UseLinkMode))
	}
	return nil
}
