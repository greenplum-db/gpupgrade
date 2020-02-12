package agent_test

import (
	"errors"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/agent"
)

type renamedPath struct {
	oldpath string
	newpath string
}

func TestReconfigureDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("it renames each pair", func(t *testing.T) {
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		renamedPaths := []renamedPath{}

		utils.System.Rename = func(oldpath, newpath string) error {
			renamedPaths = append(renamedPaths, renamedPath{oldpath: oldpath, newpath: newpath})
			return nil
		}

		agent.ReconfigureDataDirectories([]*idl.RenamePair{
			{Src: "/some/source", Dst: "/some/destination"},
			{Src: "/some/other/source", Dst: "/some/other/destination"},
		})

		if len(renamedPaths) != 2 {
			t.Errorf("got %d renames, expected %d", len(renamedPaths), 2)
		}

		assertRenamedPathsIncludes(t, renamedPaths, renamedPath{oldpath: "/some/source", newpath: "/some/destination"})
		assertRenamedPathsIncludes(t, renamedPaths, renamedPath{oldpath: "/some/other/source", newpath: "/some/other/destination"})
	})

	t.Run("it returns an error and exits without continuing when a rename fails", func(t *testing.T) {
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		renamedPaths := []renamedPath{}

		count := 0
		utils.System.Rename = func(oldpath, newpath string) error {
			count++
			if count == 1 {
				return errors.New("something")
			}

			renamedPaths = append(renamedPaths, renamedPath{oldpath: oldpath, newpath: newpath})

			t.Errorf("unexpected call to rename during ReconfigureDataDirectories")
			return nil
		}

		agent.ReconfigureDataDirectories([]*idl.RenamePair{
			{Src: "/some/source", Dst: "/some/destination"},
			{Src: "/some/other/source", Dst: "/some/other/destination"},
		})

		if len(renamedPaths) != 0 {
			t.Errorf("got %d renames, expected %d", len(renamedPaths), 2)
		}
	})
}

func assertRenamedPathsIncludes(t *testing.T, paths []renamedPath, rename renamedPath) {
	for _, path := range paths {
		if path.oldpath == rename.oldpath && path.newpath == rename.newpath {
			return
		}
	}

	t.Errorf("renamed paths %v did not include %v", paths, rename)
}
