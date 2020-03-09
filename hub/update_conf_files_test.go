package hub_test

import (
	"os/exec"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestUpdateGpperfmonConf(t *testing.T) {
	testhelper.SetupTestLogger()

	hub.SetExecCommand(exec.Command)
	defer func() { hub.SetExecCommand(nil) }()

	t.Run("it replaces the location line within the config to the new data directory location ", func(t *testing.T) {
		var usedPattern string
		var usedReplacement string
		var usedFile string

		hub.SetReplaceStringWithinFile(func(pattern string, replacement string, file string) error {
			usedPattern = pattern
			usedReplacement = replacement
			usedFile = file
			return nil
		})

		hub.UpdateGpperfmonConf("/some/master/data/dir")

		if usedPattern != "log_location = .*$" {
			t.Errorf("got %v, expected log_location = .*", usedPattern)
		}

		if usedReplacement != "log_location = /some/master/data/dir/gpperfmon/logs" {
			t.Errorf("got %v, expected %q", usedReplacement, "log_location = /some/master/data/dir/gpperfmon/logs")
		}

		if usedFile != "/some/master/data/dir/gpperfmon/conf/gpperfmon.conf" {
			t.Errorf("got %v, expected %q", usedFile, "/some/master/data/dir/gpperfmon/conf/gpperfmon.conf")
		}
	})
}

func TestUpdatePostgresqlConf(t *testing.T) {
	testhelper.SetupTestLogger()

	hub.SetExecCommand(exec.Command)
	defer func() { hub.SetExecCommand(nil) }()

	t.Run("it updates the port within the target's postgresql.conf to be the source port ", func(t *testing.T) {
		var usedPattern string
		var usedReplacement string
		var usedFile string

		hub.SetReplaceStringWithinFile(func(pattern string, replacement string, file string) error {
			usedPattern = pattern
			usedReplacement = replacement
			usedFile = file
			return nil
		})

		source = hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DataDir: "/some/source/master/data/dir", Port: 8888, Role: "p"},
		})

		target = hub.MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DataDir: "/some/target/master/data/dir", Port: 7777, Role: "p"},
		})

		hub.UpdatePostgresqlConf(9999, target, source)

		if usedPattern != "port=9999" {
			t.Errorf("got %q, expected %q", usedPattern, "port=9999")
		}

		if usedReplacement != "port=8888" {
			t.Errorf("got %q, expected %q", usedReplacement, "port=8888")
		}

		if usedFile != "/some/target/master/data/dir/postgresql.conf" {
			t.Errorf("got %q, expected %q", usedFile, "/some/target/master/data/dir/postgresql.conf")
		}
	})
}
