package hub_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestSwapDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger() // init gplog

	afterEach := func() {
		utils.System = utils.InitializeSystemFunctions()
	}

	t.Run("it renames data directories for source and target master data dirs", func(t *testing.T) {
		spy := &renameMock{}

		utils.System.Rename = spy.renameFunc()

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DataDir: "/some/data/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, DataDir: "/some/data/directory/primary1", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DataDir: "/some/qddir_upgrade/dataDirectory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, DataDir: "/some/segment1_upgrade/dataDirectory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		hub.SwapDataDirectories(hub.MakeHub(config), newAgentBrokerSpy(t))

		if spy.TimesCalled() != 2 {
			t.Errorf("got Rename called %v times, wanted %v times",
				spy.TimesCalled(),
				2)
		}

		spy.assertDirectoriesMoved(t,
			"/some/data/directory",
			"/some/data/directory_old")

		spy.assertDirectoriesMoved(t,
			"/some/qddir_upgrade/dataDirectory",
			"/some/data/directory")

		if source.Primaries[-1].DataDir != "/some/data/directory" {
			t.Errorf("got %v, wanted it to be unchanged as %v",
				source.Primaries[-1].DataDir,
				"/some/data/directory")
		}

		if target.Primaries[-1].DataDir != "/some/qddir_upgrade/dataDirectory" {
			t.Errorf("got %v, wanted it to be unchanged as %v",
				target.Primaries[-1].DataDir,
				"/some/qddir_upgrade/dataDirectory")
		}
	})

	t.Run("it returns an error if the directories cannot be renamed", func(t *testing.T) {
		defer afterEach()

		utils.System.Rename = func(oldpath, newpath string) error {
			return errors.New("failure to rename")
		}

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, DataDir: "/some/data/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, DataDir: "/some/data/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		err := hub.SwapDataDirectories(hub.MakeHub(config), newAgentBrokerSpy(t))

		if err == nil {
			t.Fatalf("got nil for an error during SwapDataDirectories, wanted a failure to move directories: %+v", err)
		}
	})

	t.Run("it does not modify the cluster state if there is an error", func(t *testing.T) {
		defer afterEach()

		utils.System.Rename = func(oldpath, newpath string) error {
			return errors.New("failure to rename")
		}

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, DataDir: "/some/data/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, DataDir: "/some/data/directory_upgrade", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		err := hub.SwapDataDirectories(hub.MakeHub(config), newAgentBrokerSpy(t))

		if err == nil {
			t.Fatalf("got nil for an error during SwapDataDirectories, wanted a failure to move directories: %+v", err)
		}

		if config.Source.Primaries[99].DataDir != "/some/data/directory" {
			t.Errorf("got new data dir of %v, wanted %v",
				config.Source.Primaries[99].DataDir, "/some/data/directory")
		}

		if config.Target.Primaries[99].DataDir != "/some/data/directory_upgrade" {
			t.Errorf("got new data dir of %v, wanted %v",
				config.Target.Primaries[99].DataDir,
				"/some/data/directory_upgrade")
		}
	})

	t.Run("it tells each agent to reconfigure data directories for the segments", func(t *testing.T) {
		spy := &renameMock{}
		utils.System.Rename = spy.renameFunc()

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, Hostname: "host1", DataDir: "/some/data/directory/99", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, Hostname: "host2", DataDir: "/some/data/directory/100", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, Hostname: "host1", DataDir: "/some/data/directory_upgrade/99", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, Hostname: "host2", DataDir: "/some/data/directory_upgrade/100", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		abSpy := newAgentBrokerSpy(t)
		h := hub.MakeHub(config)

		err := hub.SwapDataDirectories(h, abSpy)

		if err != nil {
			t.Errorf("expected no error, got %#v", err)
		}

		if abSpy.NumCalls() != 2 {
			t.Errorf("got %d, expected 2", abSpy.NumCalls())
		}

		abSpy.assertReconfigureDataDirsCalledWith(
			"host1",
			[]*idl.RenamePair{
				{
					Src: "/some/data/directory/99",
					Dst: "/some/data/directory/99_old",
				},
				{
					Src: "/some/data/directory_upgrade/99",
					Dst: "/some/data/directory/99",
				},
			},
		)

		abSpy.assertReconfigureDataDirsCalledWith(
			"host2",
			[]*idl.RenamePair{
				{
					Src: "/some/data/directory/100",
					Dst: "/some/data/directory/100_old",
				},
				{
					Src: "/some/data/directory_upgrade/100",
					Dst: "/some/data/directory/100",
				},
			},
		)
	})

	t.Run("it tells the agent for the standby master to reconfigure data directories", func(t *testing.T) {
		spy := &renameMock{}
		utils.System.Rename = spy.renameFunc()

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, Hostname: "host1", DataDir: "/some/data/master/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: -1, Hostname: "host2", DataDir: "/some/data/standby/directory", Role: utils.MirrorRole, PreferredRole: utils.MirrorRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, Hostname: "host1", DataDir: "/some/data/master_upgrade/directory", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: -1, Hostname: "host2", DataDir: "/some/data/standby_upgrade/directory", Role: utils.MirrorRole, PreferredRole: utils.MirrorRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		abSpy := newAgentBrokerSpy(t)
		h := hub.MakeHub(config)

		err := hub.SwapDataDirectories(h, abSpy)

		if err != nil {
			t.Errorf("expected no error, got %#v", err)
		}

		if abSpy.NumCalls() != 1 {
			t.Errorf("got %d, expected 1", abSpy.NumCalls())
		}

		abSpy.assertReconfigureDataDirsCalledWith(
			"host2",
			[]*idl.RenamePair{
				{
					Src: "/some/data/standby/directory",
					Dst: "/some/data/standby/directory_old",
				},
				{
					Src: "/some/data/standby_upgrade/directory",
					Dst: "/some/data/standby/directory",
				},
			},
		)
	})

	t.Run("it does not send a request to an agent if part of the rename pair is empty", func(t *testing.T) {
		spy := &renameMock{}
		utils.System.Rename = spy.renameFunc()

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, Hostname: "host1", DataDir: "/some/data/directory/99", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		abSpy := newAgentBrokerSpy(t)
		h := hub.MakeHub(config)

		err := hub.SwapDataDirectories(h, abSpy)

		if err != nil {
			t.Errorf("expected no error, got %#v", err)
		}

		if abSpy.NumCalls() != 1 {
			t.Errorf("got %d, expected 1", abSpy.NumCalls())
		}

		abSpy.assertReconfigureDataDirsCalledWith(
			"host1",
			[]*idl.RenamePair{},
		)
	})

	t.Run("it errors out if the call to the agents fails", func(t *testing.T) {
		spy := &renameMock{}
		utils.System.Rename = spy.renameFunc()

		source := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, Hostname: "host1", DataDir: "/some/data/directory/99", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, Hostname: "host2", DataDir: "/some/data/directory/100", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		target := hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 99, Hostname: "host1", DataDir: "/some/data/directory_upgrade/99", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
			{ContentID: 100, Hostname: "host2", DataDir: "/some/data/directory_upgrade/100", Role: utils.PrimaryRole, PreferredRole: utils.PrimaryRole},
		})

		config := &hub.Config{
			Source: source,
			Target: target,
		}

		abSpy := &failingAgentBroker{}
		h := hub.MakeHub(config)

		err := hub.SwapDataDirectories(h, abSpy)

		if err == nil {
			t.Errorf("got no errors from agents, expected an error for each host")
		}
	})

}

//
//
// Rename spy
//
//
type renameMock struct {
	calls []*renameCall
}

type renameCall struct {
	originalName string
	newName      string
}

func (mock *renameMock) TimesCalled() int {
	return len(mock.calls)
}

func (mock *renameMock) Call(i int) *renameCall {
	return mock.calls[i-1]
}

func (mock *renameMock) assertDirectoriesMoved(t *testing.T, originalName string, newName string) {
	var call *renameCall

	for _, c := range mock.calls {
		if c.originalName == originalName && c.newName == newName {
			call = c
			break
		}
	}

	if call == nil {
		t.Errorf("got no calls to rename %v to %v, expected 1 call", originalName, newName)
	}
}

func (mock *renameMock) renameFunc() func(oldpath string, newpath string) error {
	return func(originalName, newName string) error {
		mock.calls = append(mock.calls, &renameCall{
			originalName: originalName,
			newName:      newName,
		})

		return nil
	}
}

//
//
// Agent Broker spy
//
//
type agentBrokerSpy struct {
	t                *testing.T
	expectedHostname string
	calls            map[string][]*idl.RenamePair
	lock             sync.Mutex
}

func newAgentBrokerSpy(t *testing.T) *agentBrokerSpy {
	return &agentBrokerSpy{
		t:     t,
		calls: map[string][]*idl.RenamePair{},
	}
}

func (spy *agentBrokerSpy) ReconfigureDataDirectories(hostname string, pairs []*idl.RenamePair) error {
	spy.lock.Lock()
	defer spy.lock.Unlock()
	spy.calls[hostname] = pairs
	return nil
}

func (spy *agentBrokerSpy) assertReconfigureDataDirsCalledWith(expectedHostname string, expectedRenamePairs []*idl.RenamePair) {
	actualPairs := spy.calls[expectedHostname]

	if len(actualPairs) == 0 && len(expectedRenamePairs) == 0 {
		return
	}

	if !reflect.DeepEqual(actualPairs, expectedRenamePairs) {
		spy.t.Errorf("got no calls to agent broker for hostname %v with data dir pairs %v, actually received %+v",
			expectedHostname,
			expectedRenamePairs,
			spy.calls)
	}
}

func (spy *agentBrokerSpy) NumCalls() int {
	return len(spy.calls)
}

//
//
// Failing Agent Broker spy
//
//
type failingAgentBroker struct {
}

func (f *failingAgentBroker) ReconfigureDataDirectories(hostname string, renamePairs []*idl.RenamePair) error {
	return errors.New("hi, i'm an error")
}
