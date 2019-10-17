package services

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
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
	. "github.com/onsi/gomega"
)

func gpinitsystem() {}

func gpinitsystem_Warnings() {
	os.Stdout.WriteString("[WARN]:-Master open file limit is 256 should be >= 65535")
	os.Exit(1)
}

func gpinitsystem_Errors() {
	os.Stderr.WriteString("[ERROR]:-Failure to init")
	os.Exit(2)
}

func init() {
	exectest.RegisterMains(
		gpinitsystem,
		gpinitsystem_Warnings,
		gpinitsystem_Errors,
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
			t.Errorf("create initial gpinitsystem config failed: %v", err)
		}

		expectedConfig := []string{
			`ARRAY_NAME="gp_upgrade cluster"`,
			"SEG_PREFIX=seg",
			"TRUSTED_SHELL=ssh",
		}
		if !reflect.DeepEqual(expectedConfig, actualConfig) {
			t.Errorf("wanted: %v, got: %v", expectedConfig, actualConfig)
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
			t.Errorf("create initial gpinitsystem config failed: %v", err)
		}

		expectedConfig := []string{"CHECK_POINT_SEGMENTS=8", "ENCODING=UNICODE"}
		if !reflect.DeepEqual(expectedConfig, actualConfig) {
			t.Errorf("wanted: %v, got: %v", expectedConfig, actualConfig)
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

		actualConfig, actualSegDataDirMap, actualPort := DeclareDataDirectories([]string{}, sourceCluster)
		expectedConfig := []string{
			"QD_PRIMARY_ARRAY=localhost~15433~/data/qddir_upgrade/seg-1~1~-1~0",
			`declare -a PRIMARY_ARRAY=(
	host1~29432~/data/dbfast1_upgrade/seg1~2~0~0
	host2~29433~/data/dbfast2_upgrade/seg2~3~1~0
)`}
		if !reflect.DeepEqual(expectedConfig, actualConfig) {
			t.Errorf("wanted: %v, got: %v", expectedConfig, actualConfig)
		}

		expectedDataDirMap := map[string][]string{
			"host1": {"/data/dbfast1_upgrade"},
			"host2": {"/data/dbfast2_upgrade"},
		}
		if !reflect.DeepEqual(expectedDataDirMap, actualSegDataDirMap) {
			t.Errorf("wanted: %v, got: %v", expectedDataDirMap, actualSegDataDirMap)
		}

		expectedPort := 15433
		if expectedPort != actualPort {
			t.Errorf("wanted: %v, got: %v", expectedPort, actualPort)
		}
	})
}

func TestCreateAllDataDirectories(t *testing.T) {
	segDataDirMap := map[string][]string{
		"host1": {"/data/dbfast1_upgrade"},
		"host2": {"/data/dbfast2_upgrade"},
	}

	sourceMasterDataDir := "/data/qddir/seg-1"

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

		err := CreateAllDataDirectories([]*Connection{}, segDataDirMap, sourceMasterDataDir)
		if err != nil {
			t.Errorf("expected no error, but got: %#v", err)
		}

		expectedStatCalls := []string{"/data/qddir_upgrade"}
		if !reflect.DeepEqual(statCalls, expectedStatCalls) {
			t.Errorf("wanted: %#v, got: %#v", expectedStatCalls, statCalls)
		}

		expectedMkdirCalls := []string{"/data/qddir_upgrade"}
		if !reflect.DeepEqual(statCalls, expectedMkdirCalls) {
			t.Errorf("wanted: %#v, got: %#v", expectedMkdirCalls, statCalls)
		}
	})

	t.Run("cannot stat the master data directory", func(t *testing.T) {
		expected := errors.New("permission denied")
		utils.System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}

		err := CreateAllDataDirectories([]*Connection{}, segDataDirMap, sourceMasterDataDir)
		if !xerrors.Is(err, expected) {
			t.Errorf("wanted: %#v got: %#v", expected, err)
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

		err := CreateAllDataDirectories([]*Connection{}, segDataDirMap, sourceMasterDataDir)
		if !xerrors.Is(err, expected) {
			t.Errorf("wanted: %#v got: %#v", expected, err)
		}
	})
}

func TestCreateSegmentDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	t.Run("when gpinitsystem fails it returns an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client1 := mock_idl.NewMockAgentClient(ctrl)
		client1.EXPECT().CreateSegmentDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(&idl.CreateSegmentDataDirReply{}, nil)

		expected := errors.New("permission denied")
		client2 := mock_idl.NewMockAgentClient(ctrl)
		client2.EXPECT().CreateSegmentDataDirectories(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, expected)

		segDataDirMap := map[string][]string{
			"host1": {"/data/dbfast1_upgrade"},
			"host2": {"/data/dbfast2_upgrade"},
		}

		agentConns := []*Connection{
			{nil, client1, "host1", nil},
			{nil, client2, "host2", nil},
		}

		err := CreateSegmentDataDirectories(agentConns, segDataDirMap)
		if !xerrors.Is(err, expected) {
			t.Errorf("want: %#v got: %#v", expected, err)
		}
	})
}

func TestRunInitsystemForTargetCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	ctrl := gomock.NewController(GinkgoT())
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	mockStream.EXPECT().
		Send(gomock.Any()).
		AnyTimes()

	execCommand = nil
	defer func() {
		execCommand = nil
	}()

	targetBin := "/target/bin"
	gpinitsystemConfigPath := "/home/gpadmin/.gpupgrade/gpinitsystem_config"

	t.Run("uses the correct arguments", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("should use executables in the source's bindir even if bindir has a trailing slash", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin+"/", gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("when gpinitsystem has a warning it logs and does not return an error", func(t *testing.T) {
		_, _, log := testhelper.SetupTestLogger() // initialize gplog

		execCommand = exectest.NewCommand(gpinitsystem_Warnings)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}

		actual := string(log.Contents())
		expected := "[WARNING]:-gpinitsystem had warnings and exited with status 1"
		if strings.HasSuffix(actual, expected) {
			t.Errorf("want: %q got: %q", expected, actual)
		}
	})

	t.Run("when gpinitsystem fails it returns an error", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Errors)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)

		var actual *exec.ExitError
		if !xerrors.As(err, &actual) {
			t.Fatalf("want ExitError, but got: %#v", err)
		}

		if actual.ExitCode() != 2 {
			t.Errorf("want: 2 got: %d", actual.ExitCode())
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
			expected := "gpseg"
			if actual != expected {
				t.Errorf("wanted: %q got: %q", expected, actual)
			}
			if err != nil {
				t.Fatalf("couldn't get master segment prefix: %v", err)
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
				t.Fatalf("Expected err, but got none")
			}
		}
	})
}
