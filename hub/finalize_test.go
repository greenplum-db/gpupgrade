package hub_test

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"google.golang.org/grpc/metadata"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/cluster"

	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/hub"
)

func TestFinalize(t *testing.T) {
	numberOfSubsteps := 2

	tempDir, err := ioutil.TempDir("", "gpupgrade")
	if err != nil {
		t.Error("error creating state dir for finalize test")
	}
	defer os.RemoveAll(tempDir)

	gplog.InitializeLogging("gpupgrade agent", tempDir)

	t.Run("it upgrades the standby", func(t *testing.T) {
		hub.StubReconfigurePortsToSucceed()

		upgradeWasCalled := false
		var standbyConfigurationUsed hub.StandbyConfig
		var greenplumRunner hub.GreenplumRunner

		hub.StubUpgradeStandby(func(shellRunner hub.GreenplumRunner, config hub.StandbyConfig) error {
			upgradeWasCalled = true
			standbyConfigurationUsed = config
			greenplumRunner = shellRunner
			return nil
		})

		source = makeCluster(8888, "", "")
		target = makeCluster(9999, "some-target-hostname", "/some/target/master/data/dir")

		stream := &spyStream{}
		substepStateStore := &stubStore{}

		err = hub.Finalize(tempDir, source, target, stream, substepStateStore)

		if err != nil {
			t.Errorf("unexpected error during Finalize: %v", err)
		}

		if !upgradeWasCalled {
			t.Error("wanted the standby to be upgraded, but it was not")
		}

		if len(stream.sentMessages) == 0 {
			t.Errorf("got zero sent messages on the stream, wanted %v", numberOfSubsteps)
		}

		if !stream.messagesInclude(idl.Substep_UPGRADE_STANDBY, idl.Status_RUNNING) {
			t.Errorf("got messages %v, wanted upgrade standby is running", stream.messagesAsText())
		}

		if !stream.messagesInclude(idl.Substep_UPGRADE_STANDBY, idl.Status_COMPLETE) {
			t.Errorf("got messages %v, wanted upgrade standby is running", stream.messagesAsText())
		}

		if standbyConfigurationUsed.Port != 9999 {
			t.Errorf("got port for new standby = %v, wanted %v",
				standbyConfigurationUsed.Port, 9999)
		}

		if standbyConfigurationUsed.Hostname != "some-target-hostname" {
			t.Errorf("got hostname for new standby = %v, wanted %v",
				standbyConfigurationUsed.Hostname, "some-target-hostname")
		}

		if standbyConfigurationUsed.DataDirectory != "/some/target/master/data/dir_upgrade" {
			t.Errorf("got standby data directory for new standby = %v, wanted %v",
				standbyConfigurationUsed.DataDirectory, "/some/target/master/data/dir_upgrade")
		}

		if greenplumRunner.MasterDataDirectory() != target.MasterDataDir() {
			t.Errorf("got target cluster master data directory in greenplum runner = %v, wanted %v",
				greenplumRunner.MasterDataDirectory(), target.MasterDataDir())
		}

		if greenplumRunner.MasterPort() != target.MasterPort() {
			t.Errorf("got target cluster master port in greenplum runner = %v, wanted %v",
				greenplumRunner.MasterPort(), target.MasterPort())
		}

		if greenplumRunner.BinDir() != target.BinDir {
			t.Errorf("got target cluster master bin dir in greenplum runner = %v, wanted %v",
				greenplumRunner.BinDir(), target.BinDir)
		}
	})

	t.Run("it returns an error when upgrading the standby fails with an error", func(t *testing.T) {
		hub.StubReconfigurePortsToSucceed()

		hub.StubUpgradeStandby(func(runner hub.GreenplumRunner, config hub.StandbyConfig) error {
			return errors.New("failed")
		})

		source = makeCluster(0, "", "")
		target = makeCluster(0, "", "")
		stream := &spyStream{}
		substepStore := &stubStore{}
		substepStore.stubRead(idl.Status_UNKNOWN_STATUS, nil)

		err = hub.Finalize(tempDir, source, target, stream, substepStore)

		if err == nil {
			t.Error("got nil error for finalize, but expected it to fail during upgrade standby")
		}
	})
}

//
//
// Stub for Substep State Store
//
//
type stubStore struct {
	stubbedReadStatus idl.Status
	stubbedReadError  error
}

func (s *stubStore) Read(step idl.Substep) (idl.Status, error) {
	return s.stubbedReadStatus, s.stubbedReadError
}

func (s *stubStore) Write(idl.Substep, idl.Status) error {
	return nil
}

func (s *stubStore) stubRead(stubbedStatus idl.Status, stubbedError error) {
	s.stubbedReadStatus = stubbedStatus
	s.stubbedReadError = stubbedError
}

//
//
// Spy for Stream
//
//
type spyStream struct {
	sentMessages []*idl.Message
}

func (m *spyStream) Send(message *idl.Message) error {
	m.sentMessages = append(m.sentMessages, message)
	return nil
}

func (m *spyStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *spyStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *spyStream) SetTrailer(metadata.MD) {
}

func (m *spyStream) Context() context.Context {
	return nil
}

func (m *spyStream) SendMsg(message interface{}) error {
	return nil
}

func (m *spyStream) RecvMsg(message interface{}) error {
	return nil
}

func (m *spyStream) messagesInclude(expectedSubstep idl.Substep, expectedStatus idl.Status) bool {
	for _, message := range m.sentMessages {
		currentStatus := message.GetStatus()

		if currentStatus == nil {
			continue
		}

		if currentStatus.Step == expectedSubstep && currentStatus.Status == expectedStatus {
			return true
		}
	}
	return false
}

func (m *spyStream) messagesAsText() []string {
	var messages []string

	for _, message := range m.sentMessages {
		messages = append(messages, message.String())
	}

	return messages
}

//
//
// Dummy executor: should not have any interesting interaction with this
//
//
type dummyExecutor struct {
}

func (d *dummyExecutor) ExecuteLocalCommand(commandStr string) (string, error) {
	return "", nil
}

//
//
// Create a quick cluster of information
//
//
func makeCluster(port int, hostname string, dataDir string) *utils.Cluster {
	primaries := map[int]cluster.SegConfig{
		-1: cluster.SegConfig{
			DbID:      100,
			ContentID: -1,
			Port:      7890,
			Hostname:  "somehost",
			DataDir:   "/some/dir",
		},
	}

	mirrors := map[int]cluster.SegConfig{
		-1: cluster.SegConfig{
			DbID:      100,
			ContentID: -1,
			Port:      port,
			Hostname:  hostname,
			DataDir:   dataDir,
		},
	}

	c := &utils.Cluster{
		&cluster.Cluster{
			ContentIDs: []int{1},
			Primaries:  primaries,
			Mirrors:    mirrors,
		},
		"",
		dbconn.GPDBVersion{},
	}

	return c
}
