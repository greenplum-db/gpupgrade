package services

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
)

func gpinitsystem() {}

func gpinitsystem_Exits1() {
	os.Stdout.WriteString("[WARN]:-Master open file limit is 256 should be >= 65535")
	os.Exit(1)
}

func init() {
	exectest.RegisterMains(
		gpinitsystem,
		gpinitsystem_Exits1,
	)
}

func TestCreateInitialInitsystemConfig(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	t.Run("successfully get initial gpinitsystem config array", func(t *testing.T) {
		utils.System.Hostname = func() (string, error) {
			return "mdw", nil
		}

		actualConfig, err := CreateInitialInitsystemConfig("/data/qddir/seg-1")
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedConfig := []string{
			`ARRAY_NAME="gp_upgrade cluster"`,
			"SEG_PREFIX=seg",
			"TRUSTED_SHELL=ssh",
		}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestGetCheckpointSegmentsAndEncoding(t *testing.T) {
	t.Run("successfully get the GUC values", func(t *testing.T) {
		dbConn, sqlMock := testhelper.CreateAndConnectMockDB(1)

		checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
		encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
		sqlMock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
		sqlMock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)

		actualConfig, err := GetCheckpointSegmentsAndEncoding([]string{}, dbConn)
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedConfig := []string{"CHECK_POINT_SEGMENTS=8", "ENCODING=UNICODE"}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestDeclareDataDirectories(t *testing.T) {
	t.Run("successfully declares all directories", func(t *testing.T) {
		cluster := cluster.NewCluster([]cluster.SegConfig{
			cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1"},
			cluster.SegConfig{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1"},
			cluster.SegConfig{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2"},
		})
		sourceCluster := utils.Cluster{
			Cluster:    cluster,
			BinDir:     "/source/bindir",
			ConfigPath: "my/config/path",
			Version:    dbconn.GPDBVersion{},
		}

		actualConfig := DeclareDataDirectories([]string{}, sourceCluster)
		expectedConfig := []string{
			"QD_PRIMARY_ARRAY=localhost~15433~/data/qddir_upgrade/seg-1~1~-1~0",
			`declare -a PRIMARY_ARRAY=(
	host1~29432~/data/dbfast1_upgrade/seg1~2~0~0
	host2~29433~/data/dbfast2_upgrade/seg2~3~1~0
)`}
		if !reflect.DeepEqual(actualConfig, expectedConfig) {
			t.Errorf("got %v, want %v", actualConfig, expectedConfig)
		}
	})
}

func TestCreateAllDataDirectories(t *testing.T) {
	c := &utils.Cluster{
		Cluster: cluster.NewCluster([]cluster.SegConfig{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1"},
			{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1"},
			{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2"},
		}),
	}

	t.Run("successfully creates all directories", func(t *testing.T) {
		statCalls := []string{}
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			statCalls = append(statCalls, name)
			return nil, os.ErrNotExist
		}

		mkdirCalls := []string{}
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			mkdirCalls = append(mkdirCalls, path)
			return nil
		}

		err := CreateAllDataDirectories([]*Connection{}, c)
		if err != nil {
			t.Fatalf("got %#v, want nil", err)
		}

		expectedStatCalls := []string{"/data/qddir_upgrade"}
		if !reflect.DeepEqual(statCalls, expectedStatCalls) {
			t.Errorf("got %#v, want %#v", statCalls, expectedStatCalls)
		}

		expectedMkdirCalls := []string{"/data/qddir_upgrade"}
		if !reflect.DeepEqual(mkdirCalls, expectedMkdirCalls) {
			t.Errorf("got %#v, want %#v", mkdirCalls, expectedMkdirCalls)
		}
	})

	t.Run("cannot stat the master data directory", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}

		err := CreateAllDataDirectories([]*Connection{}, c)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v, want %#v", err, expected)
		}
	})

	t.Run("cannot create the master data directory", func(t *testing.T) {
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		expected := errors.New("permission denied")
		utils.System.MkdirAll = func(path string, perm os.FileMode) error {
			return expected
		}

		err := CreateAllDataDirectories([]*Connection{}, c)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v, want %#v", err, expected)
		}
	})
}

func TestCreateSegmentDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c := &utils.Cluster{
		Cluster: cluster.NewCluster([]cluster.SegConfig{
			{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1"},
			{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1"},
			{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2"},
		}),
	}

	client := mock_idl.NewMockAgentClient(ctrl)
	client.EXPECT().CreateSegmentDataDirectories(
		gomock.Any(),
		&idl.CreateSegmentDataDirRequest{
			Datadirs: []string{"/data/dbfast1_upgrade"},
		},
	).Return(&idl.CreateSegmentDataDirReply{}, nil)

	expected := errors.New("permission denied")
	failedClient := mock_idl.NewMockAgentClient(ctrl)
	failedClient.EXPECT().CreateSegmentDataDirectories(
		gomock.Any(),
		&idl.CreateSegmentDataDirRequest{
			Datadirs: []string{"/data/dbfast2_upgrade"},
		},
	).Return(nil, expected)

	agentConns := []*Connection{
		{nil, client, "host1", nil},
		{nil, failedClient, "host2", nil},
	}

	err := CreateSegmentDataDirectories(agentConns, c)
	if !xerrors.Is(err, expected) {
		t.Errorf("got %#v, want %#v", err, expected)
	}
}

func TestRunInitsystemForTargetCluster(t *testing.T) {
	ctrl := gomock.NewController(GinkgoT())
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	mockStream.EXPECT().
		Send(gomock.Any()).
		AnyTimes()

	cluster6X := &utils.Cluster{
		BinDir: "/target/bin",
		Version: dbconn.NewVersion("6.0.0"),
	}

	cluster7X := &utils.Cluster{
		BinDir: "/target/bin",
		Version: dbconn.NewVersion("7.0.0"),
	}

	gpinitsystemConfigPath := "/dir/.gpupgrade/gpinitsystem_config"

	execCommand = nil
	defer func() {
		execCommand = nil
	}()

	t.Run("does not use --ignore-warnings when upgrading to GPDB7 or higher", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("executed %q, want bash", path)
				}

				expected := []string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /dir/.gpupgrade/gpinitsystem_config"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("args %q, want %q", args, expected)
				}
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, cluster7X, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("only uses --ignore-warnings when upgrading to GPDB6", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("executed %q, want bash", path)
				}

				expected := []string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /dir/.gpupgrade/gpinitsystem_config --ignore-warnings"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("args %q, want %q", args, expected)
				}
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, cluster6X, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("should use executables in the source's bindir even if bindir has a trailing slash", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				if path != "bash" {
					t.Errorf("executed %q, want bash", path)
				}

				expected := []string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /dir/.gpupgrade/gpinitsystem_config"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("args %q, want %q", args, expected)
				}
			})

		cluster7X.BinDir += "/"
		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, cluster7X, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("returns an error when gpinitsystem fails with --ignore-warnings when upgrading to GPDB6", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Exits1)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, cluster6X, gpinitsystemConfigPath)

		var actual *exec.ExitError
		if !xerrors.As(err, &actual) {
			t.Fatalf("got %#v, want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d, want 1 ", actual.ExitCode())
		}
	})

	t.Run("returns an error when gpinitsystem errors when upgrading to GPDB7 or higher", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Exits1)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, cluster7X, gpinitsystemConfigPath)

		var actual *exec.ExitError
		if !xerrors.As(err, &actual) {
			t.Fatalf("got %#v, want ExitError", err)
		}

		if actual.ExitCode() != 1 {
			t.Errorf("got %d, want 1", actual.ExitCode())
		}
	})
}

func TestGetMasterSegPrefix(t *testing.T) {
	t.Run("returns a valid seg prefix given", func(t *testing.T) {
		cases := []struct {
			desc          string
			MasterDataDir string
		}{
			{"an absolute path", "/data/master/gpseg-1"},
			{"a relative path", "../master/gpseg-1"},
			{"a implicitly relative path", "gpseg-1"},
		}

		for _, c := range cases {
			actual, err := GetMasterSegPrefix(c.MasterDataDir)
			if err != nil {
				t.Fatalf("got %#v, want nil", err)
			}

			expected := "gpseg"
			if actual != expected {
				t.Errorf("got %q, want %q", actual, expected)
			}
		}
	})

	t.Run("returns errors when given", func(t *testing.T) {
		cases := []struct {
			desc          string
			MasterDataDir string
		}{
			{"the empty string", ""},
			{"a path without a content identifier", "/opt/myseg"},
			{"a path with a segment content identifier", "/opt/myseg2"},
			{"a path that is only a content identifier", "-1"},
			{"a path that ends in only a content identifier", "///-1"},
		}

		for _, c := range cases {
			_, err := GetMasterSegPrefix(c.MasterDataDir)
			if err == nil {
				t.Fatalf("got nil, want err")
			}
		}
	})
}
