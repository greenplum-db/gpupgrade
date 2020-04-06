package agent_test

import (
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestDeleteDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	server := agent.NewServer(agent.Config{
		Port:     -1,
		StateDir: "",
	})

	cases := []struct {
		name             string
		checkIdempotence bool
	}{
		{"does delete directories",
			false},
		{"is idempotent",
			true},
	}
	for _, c := range cases {

		t.Run(c.name, func(t *testing.T) {
			dataDir1, dataDir2, tmpDir := testutils.SetupDataDirs(t)
			defer func() {
				os.RemoveAll(tmpDir)
			}()

			expectedDataDirectories := []string{dataDir1, dataDir2}
			req := &idl.DeleteDirectoriesRequest{Datadirs: expectedDataDirectories}

			_, err := server.DeleteDirectories(context.Background(), req)
			if err != nil {
				t.Errorf("unexpected error got %+v", err)
			}
			if utils.DoesPathExist(dataDir1) {
				t.Errorf("expected false")
			}
			if utils.DoesPathExist(dataDir2) {
				t.Errorf("expected false")
			}

			if c.checkIdempotence {
				_, err = server.DeleteDirectories(context.Background(), req)
				if err != nil {
					t.Errorf("unexpected error got %+v", err)
				}
			}
		})
	}

	t.Run("correctly reports failure of deleting a data directory", func(t *testing.T) {
		dataDir1, dataDir2, tmpDir := testutils.SetupDataDirs(t)
		defer func() {
			os.RemoveAll(tmpDir)
		}()

		expected := &os.PathError{Err: syscall.EPERM}
		expectedDataDirectories := []string{dataDir1, dataDir2}
		req := &idl.DeleteDirectoriesRequest{Datadirs: expectedDataDirectories}

		actualDataDirectories := []string{}
		utils.System.RemoveAll = func(name string) error {
			actualDataDirectories = append(actualDataDirectories, name)
			if name == dataDir2 {
				return expected
			}
			return nil
		}
		defer func() {
			utils.System.RemoveAll = os.RemoveAll
		}()

		_, err := server.DeleteDirectories(context.Background(), req)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("got %d errors, want %d", len(multiErr.Errors), 1)
		}

		if !reflect.DeepEqual(actualDataDirectories, expectedDataDirectories) {
			t.Errorf("got %s, want %s", actualDataDirectories, expectedDataDirectories)
		}

		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})

}
