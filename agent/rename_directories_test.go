package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestRenameDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	server := agent.NewServer(agent.Config{
		Port:     -1,
		StateDir: "",
	})

	pair := idl.RenamePair{
		Src: "/data/dbfast1_upgrade",
		Dst: "/data/dbfast1",
	}

	req := &idl.RenameDirectoriesRequest{
		Pairs: []*idl.RenamePair{&pair},
	}

	t.Run("successfully renames src and dst data directories", func(t *testing.T) {
		utils.System.Rename = func(src, dst string) error {
			if src != pair.Src {
				t.Errorf("got %q want %q", src, pair.Src)
			}

			if dst != pair.Dst {
				t.Errorf("got %q want %q", dst, pair.Dst)
			}

			return nil
		}

		_, err := server.RenameDirectories(context.Background(), req)
		if err != nil {
			t.Errorf("unexpected error got %#v", err)
		}
	})

	t.Run("fails to rename src and dst data directories", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Rename = func(src, dst string) error {
			return expected
		}

		_, err := server.RenameDirectories(context.Background(), req)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v want %#v", err, expected)
		}
	})
}
