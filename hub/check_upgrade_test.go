// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/grpc"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
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

var connections = []*idl.Connection{{Conn: nil, Hostname: "bengie"}}

func setUpgrader(updated UpgradeChecker) {
	upgrader = updated
}
func resetUpgrader() {
	upgrader = upgradeChecker{}
}

func TestMasterIsCheckedLinkModeTrue(t *testing.T) {
	source := MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})
	intermediate := MustCreateCluster(t, greenplum.SegConfigs{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: greenplum.PrimaryRole},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: greenplum.PrimaryRole},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: greenplum.PrimaryRole},
	})
	var stateDirExpected = "/some/state/dir"

	for _, linkMode := range []bool{true, false} {
		t.Run(fmt.Sprintf("check upgrade correctly passes useLinkMode is %v", linkMode), func(t *testing.T) {
			conf := &Config{
				Source:       source,
				Intermediate: intermediate,
				UseLinkMode:  linkMode,
			}
			s := New(conf, grpc.DialContext, stateDirExpected)
			testUpgraderMock := upgraderMock{s}

			setUpgrader(testUpgraderMock)
			defer resetUpgrader()

			err := s.CheckUpgrade(nil, connections)

			if err != nil {
				t.Errorf("got error: %+v", err) // yes, '%+v'; '%#v' prints opaque multiple errors
			}
		})
	}
}

func UpgradeMasterMock(result UpgradeMasterArgs, expected *Server) error {
	if !reflect.DeepEqual(result.Source, expected.Source) {
		return fmt.Errorf("got %#v, expected %#v", result.Source, expected.Source)
	}
	if !reflect.DeepEqual(result.Intermediate, expected.Intermediate) {
		return fmt.Errorf("got %#v, expected %#v", result.Intermediate, expected.Intermediate)
	}
	if result.StateDir != expected.StateDir {
		return fmt.Errorf("got %#v expected %#v", result.StateDir, expected.StateDir)
	}
	// does not seem worth testing stream right now
	if result.CheckOnly != true {
		return fmt.Errorf("got %#v expected %#v", result.CheckOnly, true)
	}
	if result.UseLinkMode != expected.UseLinkMode {
		return fmt.Errorf("got %#v expected %#v", result.UseLinkMode, expected.UseLinkMode)
	}
	return nil
}

func UpgradePrimariesMock(result UpgradePrimaryArgs, expected *Server) error {
	if result.CheckOnly != true {
		return fmt.Errorf("got %#v expected %#v", result.CheckOnly, true)
	}
	if result.MasterBackupDir != "" {
		return fmt.Errorf("got %#v expected %#v", result.MasterBackupDir, "")
	}
	if !reflect.DeepEqual(result.AgentConns, connections) {
		return fmt.Errorf("got %#v expected %#v", result.AgentConns, connections)
	}
	expectedDataDirs, _ := expected.GetDataDirPairs()
	if !reflect.DeepEqual(result.DataDirPairMap, expectedDataDirs) {
		return fmt.Errorf("got %#v expected %#v", result.DataDirPairMap, expectedDataDirs)
	}
	if !reflect.DeepEqual(result.Source, expected.Source) {
		return fmt.Errorf("got %#v, expected %#v", result.Source, expected.Source)
	}
	if !reflect.DeepEqual(result.Intermediate, expected.Intermediate) {
		return fmt.Errorf("got %#v, expected %#v", result.Intermediate, expected.Intermediate)
	}
	if result.UseLinkMode != expected.UseLinkMode {
		return fmt.Errorf("got %#v expected %#v", result.UseLinkMode, expected.UseLinkMode)
	}
	return nil
}
