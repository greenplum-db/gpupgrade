package hub

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
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

func TestWriteSegmentArray(t *testing.T) {
	test := func(t *testing.T, cluster *utils.Cluster, ports []uint32, expected []string) {
		t.Helper()

		actual, masterPort, err := WriteSegmentArray([]string{}, cluster, ports)
		if err != nil {
			t.Errorf("got %#v", err)
		}

		expectedPort := uint32(50432)
		if len(ports) > 0 {
			expectedPort = ports[0]
		}

		if uint32(masterPort) != expectedPort {
			t.Errorf("returned master port %d, want %d", masterPort, expectedPort)
		}

		if !reflect.DeepEqual(actual, expected) {
			// Help developers see differences between the lines.
			pretty := func(lines []string) string {
				b := new(strings.Builder)

				fmt.Fprintln(b, "[")
				for _, l := range lines {
					fmt.Fprintf(b, "  %q\n", l)
				}
				fmt.Fprint(b, "]")

				return b.String()
			}
			t.Errorf("got %v, want %v", pretty(actual), pretty(expected))
		}
	}

	t.Run("correctly chooses ports when the master and segments are all on different hosts", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw2", DataDir: "/data/dbfast2/seg2", Role: "p"},
		})
		ports := []uint32{15433, 15434}

		test(t, cluster, ports, []string{
			"QD_PRIMARY_ARRAY=mdw~15433~/data/qddir_upgrade/seg-1~1~-1~0",
			"declare -a PRIMARY_ARRAY=(",
			"\tsdw1~15434~/data/dbfast1_upgrade/seg1~2~0~0",
			"\tsdw2~15434~/data/dbfast2_upgrade/seg2~3~1~0",
			")",
		})
	})

	t.Run("correctly chooses ports when the master is on one host and segments on another", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		})
		ports := []uint32{15433, 25432, 25433}

		test(t, cluster, ports, []string{
			"QD_PRIMARY_ARRAY=mdw~15433~/data/qddir_upgrade/seg-1~1~-1~0",
			"declare -a PRIMARY_ARRAY=(",
			"\tsdw1~25432~/data/dbfast1_upgrade/seg1~2~0~0",
			"\tsdw1~25433~/data/dbfast2_upgrade/seg2~3~1~0",
			")",
		})
	})

	t.Run("sorts and deduplicates provided port range", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		})
		ports := []uint32{10, 9, 10, 9, 10, 8}

		test(t, cluster, ports, []string{
			"QD_PRIMARY_ARRAY=mdw~8~/data/qddir_upgrade/seg-1~1~-1~0",
			"declare -a PRIMARY_ARRAY=(",
			"\tmdw~9~/data/dbfast1_upgrade/seg1~2~0~0",
			"\tmdw~10~/data/dbfast2_upgrade/seg2~3~1~0",
			")",
		})
	})

	t.Run("uses default port range when port list is empty", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
			{ContentID: 2, DbID: 4, Hostname: "sdw1", DataDir: "/data/dbfast3/seg3", Role: "p"},
		})

		test(t, cluster, []uint32{}, []string{
			"QD_PRIMARY_ARRAY=mdw~50432~/data/qddir_upgrade/seg-1~1~-1~0",
			"declare -a PRIMARY_ARRAY=(",
			"\tmdw~50433~/data/dbfast1_upgrade/seg1~2~0~0",
			"\tmdw~50434~/data/dbfast2_upgrade/seg2~3~1~0",
			"\tsdw1~50433~/data/dbfast3_upgrade/seg3~4~2~0",
			")",
		})
	})

	t.Run("errors when old cluster contains no master segment", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		})
		ports := []uint32{15433}

		_, _, err := WriteSegmentArray([]string{}, cluster, ports)
		if err == nil {
			t.Errorf("expected error got nil")
		}
	})

	t.Run("errors when not given enough ports (single host)", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "mdw", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "mdw", DataDir: "/data/dbfast2/seg2", Role: "p"},
		})
		ports := []uint32{15433}

		_, _, err := WriteSegmentArray([]string{}, cluster, ports)
		if err == nil {
			t.Errorf("expected error got nil")
		}
	})

	t.Run("errors when not given enough ports (multiple hosts)", func(t *testing.T) {
		cluster := MustCreateCluster(t, []utils.SegConfig{
			{ContentID: -1, DbID: 1, Hostname: "mdw", DataDir: "/data/qddir/seg-1", Role: "p"},
			{ContentID: 0, DbID: 2, Hostname: "sdw1", DataDir: "/data/dbfast1/seg1", Role: "p"},
			{ContentID: 1, DbID: 3, Hostname: "sdw1", DataDir: "/data/dbfast2/seg2", Role: "p"},
		})
		ports := []uint32{15433, 25432}

		_, _, err := WriteSegmentArray([]string{}, cluster, ports)
		if err == nil {
			t.Errorf("expected error got nil")
		}
	})
}

func TestCreateSegmentDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c := MustCreateCluster(t, []utils.SegConfig{
		{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "/data/qddir/seg-1", Role: "p"},
		{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "/data/dbfast1/seg1", Role: "p"},
		{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "/data/dbfast2/seg2", Role: "p"},
	})

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
	cluster6X := &utils.Cluster{
		BinDir:  "/target/bin",
		Version: dbconn.NewVersion("6.0.0"),
	}

	cluster7X := &utils.Cluster{
		BinDir:  "/target/bin",
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

		err := RunInitsystemForTargetCluster(DevNull, cluster7X, gpinitsystemConfigPath)
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

		err := RunInitsystemForTargetCluster(DevNull, cluster6X, gpinitsystemConfigPath)
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
		err := RunInitsystemForTargetCluster(DevNull, cluster7X, gpinitsystemConfigPath)
		if err != nil {
			t.Error("gpinitsystem failed")
		}
	})

	t.Run("returns an error when gpinitsystem fails with --ignore-warnings when upgrading to GPDB6", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Exits1)

		err := RunInitsystemForTargetCluster(DevNull, cluster6X, gpinitsystemConfigPath)

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

		err := RunInitsystemForTargetCluster(DevNull, cluster7X, gpinitsystemConfigPath)

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
