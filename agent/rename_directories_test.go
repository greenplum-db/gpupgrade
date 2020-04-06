package agent_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/upgrade"

	"github.com/greenplum-db/gpupgrade/testutils"

	"github.com/greenplum-db/gpupgrade/hub"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
)

func TestRenameDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("calls the intended functions to ensure delegation to hub.RenameDataDirs", func(t *testing.T) {

		numPairs := 5

		// set up expectations
		var expectedSourceDirs, expectedTargetDirs, tmpDirs []string
		var expectedUpgradeIDs []upgrade.ID
		var renamePairs []*idl.RenameDataDirs
		for i := 0; i < numPairs; i++ {
			source, target, tmpDir := testutils.SetupDataDirs(t)
			expectedSourceDirs = append(expectedSourceDirs, source)
			if i == numPairs-1 {
				// test the use case of a target of ""(target mirror/standby doesn't exist)
				expectedTargetDirs = append(expectedTargetDirs, "")
			} else {
				expectedTargetDirs = append(expectedTargetDirs, target)
			}
			expectedUpgradeIDs = append(expectedUpgradeIDs, testutils.UpgradeID)
			tmpDirs = append(tmpDirs, tmpDir)
			renamePairs = append(renamePairs,
				&idl.RenameDataDirs{
					Source: expectedSourceDirs[i],
					Target: expectedTargetDirs[i],
				})
		}
		// rename gets call with all args, but IsPostgres not with a target of ""
		expectedTargetDirsIsPostgres := expectedTargetDirs[:len(expectedTargetDirs)-1]

		defer func() {
			for _, tmpDir := range tmpDirs {
				os.RemoveAll(tmpDir)
			}
		}()

		// set up function spies
		numCalls := 0
		var fileCheckSourceDirs, fileCheckTargetDirs []string
		agent.IsPostgresFunc = func(dataDir string) bool {
			if numCalls%2 == 0 {
				fileCheckSourceDirs = append(fileCheckSourceDirs, dataDir)
			} else if numCalls%2 == 1 {
				fileCheckTargetDirs = append(fileCheckTargetDirs, dataDir)
			}
			numCalls++
			return true
		}

		var renameSourceDirs, renameTargetDirs []string
		var renameUpgradeIDs []upgrade.ID
		agent.RenameDataDirsFunc = func(source, target string, upgradeID upgrade.ID) error {
			renameSourceDirs = append(renameSourceDirs, source)
			renameTargetDirs = append(renameTargetDirs, target)
			renameUpgradeIDs = append(renameUpgradeIDs, upgradeID)
			return nil
		}

		defer func() {
			agent.IsPostgresFunc = agent.IsPostgres
			agent.RenameDataDirsFunc = hub.RenameDataDirs
		}()

		// make the call on the agent
		server := agent.NewServer(agent.Config{
			Port:     -1,
			StateDir: "",
		})

		req := &idl.RenameDataDirectoriesRequest{
			DataDirs:  renamePairs,
			UpgradeID: uint64(testutils.UpgradeID),
		}

		_, err := server.RenameDataDirectories(context.Background(), req)
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}

		// validate results
		if !reflect.DeepEqual(expectedSourceDirs, fileCheckSourceDirs) {
			t.Errorf("expected %v got %v", expectedSourceDirs, fileCheckSourceDirs)
		}
		if !reflect.DeepEqual(expectedTargetDirsIsPostgres, fileCheckTargetDirs) {
			t.Errorf("expected %v got %v", expectedTargetDirsIsPostgres, fileCheckTargetDirs)
		}

		if !reflect.DeepEqual(expectedSourceDirs, renameSourceDirs) {
			t.Errorf("expected %v got %v", expectedSourceDirs, renameSourceDirs)
		}
		if !reflect.DeepEqual(expectedTargetDirs, renameTargetDirs) {
			t.Errorf("expected %v got %v", expectedTargetDirs, renameTargetDirs)
		}
		if !reflect.DeepEqual(expectedUpgradeIDs, renameUpgradeIDs) {
			t.Errorf("expected %v got %v", expectedUpgradeIDs, renameUpgradeIDs)
		}

	})

	t.Run("is idempotent", func(t *testing.T) {
		source, target, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		// make the call on the agent
		server := agent.NewServer(agent.Config{
			Port:     -1,
			StateDir: "",
		})

		req := &idl.RenameDataDirectoriesRequest{
			DataDirs: []*idl.RenameDataDirs{
				{
					Source: source,
					Target: target,
				},
			},
			UpgradeID: uint64(testutils.UpgradeID),
		}

		_, err := server.RenameDataDirectories(context.Background(), req)
		if err != nil {
			t.Errorf("unexpected error got %v", err)
		}
		if !hub.AlreadyRenamed(source, target, upgrade.ArchiveDirectoryForSource(source, testutils.UpgradeID), false) {
			t.Errorf("expected true")
		}

		_, err = server.RenameDataDirectories(context.Background(), req)
		if err != nil {
			t.Errorf("unexpected error got %v", err)
		}
		if !hub.AlreadyRenamed(source, target, upgrade.ArchiveDirectoryForSource(source, testutils.UpgradeID), false) {
			t.Errorf("expected true")
		}

	})
}
