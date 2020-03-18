package agent_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/agent"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestDeleteMirrorAndStandbyDirectories(t *testing.T) {
	testhelper.SetupTestLogger()

	server := agent.NewServer(agent.Config{
		Port:     -1,
		StateDir: "",
	})

	req := &idl.DeleteDirectoriesRequest{Datadirs: []string{"/data/dbfast_mirror1/seg1", "/data/dbfast_mirror2/seg2"}}

	t.Run("successfully deletes the data directories", func(t *testing.T) {
		expectedDataDirectories := []string{"/data/dbfast_mirror1/seg1", "/data/dbfast_mirror2/seg2"}
		actualDataDirectories :=  []string{}
		utils.System.RemoveAll = func(name string) error {
			actualDataDirectories = append(actualDataDirectories, name)
			return nil
		}

		expectedFilesStatCalls := []string{"/data/dbfast_mirror1/seg1/postgresql.conf",
			"/data/dbfast_mirror1/seg1/PG_VERSION",
			"/data/dbfast_mirror2/seg2/postgresql.conf",
			"/data/dbfast_mirror2/seg2/PG_VERSION"}
		actualFilesStatCalls :=  []string{}
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			actualFilesStatCalls = append(actualFilesStatCalls, name)

			return nil, nil
		}

		_, err := server.DeleteDirectories(context.Background(), req)

		if !reflect.DeepEqual(actualDataDirectories, expectedDataDirectories) {
			t.Errorf("got %s, want %s", actualDataDirectories, expectedDataDirectories)
		}

		if !reflect.DeepEqual(actualFilesStatCalls, expectedFilesStatCalls) {
			t.Errorf("got %s, want %s", actualFilesStatCalls, expectedFilesStatCalls)
		}

		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}
	})

	t.Run("fails to open configuration files under segment data directory", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}

		_, err := server.DeleteDirectories(context.Background(), req)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 4 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})

	t.Run("fails to remove one segment data directory", func(t *testing.T) {
		expected := errors.New("permission denied")
		expectedDataDirectories := []string{"/data/dbfast_mirror1/seg1", "/data/dbfast_mirror2/seg2"}
		actualDataDirectories :=  []string{}
		utils.System.RemoveAll = func(name string) error {
			actualDataDirectories = append(actualDataDirectories, name)
			if name == "/data/dbfast_mirror1/seg1" {
				return expected
			}
			return nil
		}

		statCalls := 0
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			statCalls++
			return nil, nil
		}

		_, err := server.DeleteDirectories(context.Background(), req)

		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("got %d errors, want %d", len(multiErr.Errors), 1)
		}

		if statCalls != 4 {
			t.Errorf("got %d stat calls, want 4", statCalls)
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
