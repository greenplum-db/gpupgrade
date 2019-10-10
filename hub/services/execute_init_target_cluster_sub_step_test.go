package services

import (
	"os"

	"database/sql/driver"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hub prepare init-cluster", func() {

	Describe("CreateInitialInitsystemConfig", func() {
		It("successfully get initial gpinitsystem config array", func() {
			utils.System.Hostname = func() (string, error) {
				return "mdw", nil
			}
			expectedConfig := []string{
				`ARRAY_NAME="gp_upgrade cluster"`, "SEG_PREFIX=seg",
				"TRUSTED_SHELL=ssh"}
			gpinitsystemConfig, err := CreateInitialInitsystemConfig("/data/seg-1")
			Expect(err).To(BeNil())
			Expect(gpinitsystemConfig).To(Equal(expectedConfig))
		})
	})

	Describe("GetCheckpointSegmentsAndEncoding", func() {
		It("successfully get the GUC values", func() {

			dbConnector, sqlMock := testhelper.CreateAndConnectMockDB(1)


			checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
			encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
			sqlMock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
			sqlMock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)
			expectedConfig := []string{"CHECK_POINT_SEGMENTS=8", "ENCODING=UNICODE"}
			gpinitsystemConfig, err := GetCheckpointSegmentsAndEncoding([]string{}, dbConnector)
			Expect(err).To(BeNil())
			Expect(gpinitsystemConfig).To(Equal(expectedConfig))
		})
	})

	Describe("DeclareDataDirectories", func() {
		It("successfully declares all directories", func() {
			cluster := cluster.NewCluster([]cluster.SegConfig{
				cluster.SegConfig{ContentID: -1, DbID: 1, Port: 15432, Hostname: "localhost", DataDir: "basedir/seg-1"},
				cluster.SegConfig{ContentID: 0, DbID: 2, Port: 25432, Hostname: "host1", DataDir: "basedir/seg1"},
				cluster.SegConfig{ContentID: 1, DbID: 3, Port: 25433, Hostname: "host2", DataDir: "basedir/seg2"},
			})
			source := &utils.Cluster{
				Cluster:    cluster,
				BinDir:     "/source/bindir",
				ConfigPath: "my/config/path",
				Version:    dbconn.GPDBVersion{},
			}

			resultConfig, resultMap, port := DeclareDataDirectories(source, []string{})

			expectedConfig := []string{"QD_PRIMARY_ARRAY=localhost~15433~basedir_upgrade/seg-1~1~-1~0",
				`declare -a PRIMARY_ARRAY=(
	host1~29432~basedir_upgrade/seg1~2~0~0
	host2~29433~basedir_upgrade/seg2~3~1~0
)`}

			Expect(resultConfig).To(Equal(expectedConfig))
			Expect(resultMap).To(Equal(map[string][]string{
				"host1": {"basedir_upgrade"},
				"host2": {"basedir_upgrade"},
			}))
			Expect(port).To(Equal(15433))
		})
	})

	Describe("CreateAllDataDirectories", func() {
		It("successfully creates all directories", func() {
			statCalls := []string{}
			mkdirCalls := []string{}
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				statCalls = append(statCalls, name)
				return nil, os.ErrNotExist
			}
			utils.System.MkdirAll = func(path string, perm os.FileMode) error {
				mkdirCalls = append(mkdirCalls, path)
				return nil
			}
			fakeConns := []*Connection{}
			segDataDirMap := map[string][]string{
				"host1": {"basedir_upgrade"},
				"host2": {"basedir_upgrade"},
			}
			err := CreateAllDataDirectories("/data/seg-1", fakeConns, segDataDirMap)
			Expect(err).To(BeNil())
			Expect(statCalls).To(Equal([]string{"/data_upgrade"}))
			Expect(mkdirCalls).To(Equal([]string{"/data_upgrade"}))
		})

		It("cannot stat the master data directory", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, errors.New("permission denied")
			}
			fakeConns := []*Connection{}
			segDataDirMap := map[string][]string{
				"host1": {"basedir_upgrade"},
				"host2": {"basedir_upgrade"},
			}
			expectedErr := errors.Errorf("Error statting new directory /data_upgrade: permission denied")
			err := CreateAllDataDirectories("/data/seg-1", fakeConns, segDataDirMap)
			Expect(err.Error()).To(Equal(expectedErr.Error()))
		})

		It("cannot create the master data directory", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}
			utils.System.MkdirAll = func(path string, perm os.FileMode) error {
				return errors.New("permission denied")
			}
			fakeConns := []*Connection{}
			segDataDirMap := map[string][]string{
				"host1": {"basedir_upgrade"},
				"host2": {"basedir_upgrade"},
			}
			expectedErr := errors.New("Could not create new directory: permission denied")
			err := CreateAllDataDirectories("/data/seg-1", fakeConns, segDataDirMap)
			Expect(err.Error()).To(Equal(expectedErr.Error()))
		})
	})

	Describe("RunInitsystemForTargetCluster", func() {
		var ctrl       *gomock.Controller
		var mockStream *mock_idl.MockCliToHub_ExecuteServer

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockStream = mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		})

		It("uses the correct arguments", func() {
			execCommand = exectest.NewCommandWithVerifier(EmptyMain,
				func(path string, args ...string) {
					Expect(path).To(Equal("bash"))
					Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && /target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
				})

			err := RunInitsystemForTargetCluster(mockStream, "/home/gpadmin/.gpupgrade/gpinitsystem_config", "/target/bindir/")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should use executables in the source's bindir even if bindir has a trailing slash", func() {
			execCommand = exectest.NewCommandWithVerifier(EmptyMain,
				func(path string, args ...string) {
					Expect(path).To(Equal("bash"))
					Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && /target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
				})

			err := RunInitsystemForTargetCluster(mockStream, "/home/gpadmin/.gpupgrade/gpinitsystem_config", "/target/bindir/")
			Expect(err).ToNot(HaveOccurred())
		})

		It("gpinitsystem fails", func() {
			execCommand = exectest.NewCommand(FailedMain)

			err := RunInitsystemForTargetCluster(mockStream, "/home/gpadmin/.gpupgrade/gpinitsystem_config", "/target/bindir/")
			Expect(err.Error()).To(Equal("exit status 1"))
		})
	})

	Describe("GetMasterSegPrefix", func() {
		DescribeTable("returns a valid seg prefix given",
			func(datadir string) {
				segPrefix, err := GetMasterSegPrefix(datadir)
				Expect(segPrefix).To(Equal("gpseg"))
				Expect(err).ShouldNot(HaveOccurred())
			},
			Entry("an absolute path", "/data/master/gpseg-1"),
			Entry("a relative path", "../master/gpseg-1"),
			Entry("a implicitly relative path", "gpseg-1"),
		)

		DescribeTable("returns errors when given",
			func(datadir string) {
				_, err := GetMasterSegPrefix(datadir)
				Expect(err).To(HaveOccurred())
			},
			Entry("the empty string", ""),
			Entry("a path without a content identifier", "/opt/myseg"),
			Entry("a path with a segment content identifier", "/opt/myseg2"),
			Entry("a path that is only a content identifier", "-1"),
			Entry("a path that ends in only a content identifier", "///-1"),
		)
	})

})
