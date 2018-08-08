package services_test

import (
	"database/sql/driver"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
)

var _ = Describe("Hub prepare init-cluster", func() {
	var (
		segDataDirMap map[string][]string
	)

	BeforeEach(func() {
		segDataDirMap = map[string][]string{
			"host1": {fmt.Sprintf("%s_upgrade", dir)},
			"host2": {fmt.Sprintf("%s_upgrade", dir)},
		}

		cm := testutils.NewMockChecklistManager()
		hub = services.NewHub(source, target, grpc.DialContext, hubConf, cm)
	})

	Describe("CreateInitialInitsystemConfig", func() {
		It("successfully get initial gpinitsystem config array", func() {
			utils.System.Hostname = func() (string, error) {
				return "mdw", nil
			}
			expectedConfig := []string{
				`ARRAY_NAME="gp_upgrade cluster"`, "SEG_PREFIX=seg",
				"TRUSTED_SHELL=ssh"}
			gpinitsystemConfig, err := hub.CreateInitialInitsystemConfig()
			Expect(err).To(BeNil())
			Expect(gpinitsystemConfig).To(Equal(expectedConfig))
		})
	})
	Describe("GetCheckpointSegmentsAndEncoding", func() {
		It("successfully get the GUC values", func() {
			checkpointRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("8"))
			encodingRow := sqlmock.NewRows([]string{"string"}).AddRow(driver.Value("UNICODE"))
			mock.ExpectQuery("SELECT .*checkpoint.*").WillReturnRows(checkpointRow)
			mock.ExpectQuery("SELECT .*server.*").WillReturnRows(encodingRow)
			expectedConfig := []string{"CHECK_POINT_SEGMENTS=8", "ENCODING=UNICODE"}
			gpinitsystemConfig, err := services.GetCheckpointSegmentsAndEncoding([]string{}, dbConnector)
			Expect(err).To(BeNil())
			Expect(gpinitsystemConfig).To(Equal(expectedConfig))
		})
	})

	Describe("DeclareDataDirectories", func() {
		It("successfully declares all directories", func() {
			expectedConfig := []string{fmt.Sprintf("QD_PRIMARY_ARRAY=localhost~15433~%[1]s_upgrade/seg-1~1~-1~0", dir),
				fmt.Sprintf(`declare -a PRIMARY_ARRAY=(
	host1~27432~%[1]s_upgrade/seg1~2~0~0
	host2~27433~%[1]s_upgrade/seg2~3~1~0
)`, dir)}
			resultConfig, resultMap := hub.DeclareDataDirectories([]string{})
			Expect(resultMap).To(Equal(segDataDirMap))
			Expect(resultConfig).To(Equal(expectedConfig))
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
			fakeConns := []*services.Connection{}
			err := hub.CreateAllDataDirectories(fakeConns, segDataDirMap)
			Expect(err).To(BeNil())
			Expect(statCalls).To(Equal([]string{fmt.Sprintf("%s_upgrade", dir)}))
			Expect(mkdirCalls).To(Equal([]string{fmt.Sprintf("%s_upgrade", dir)}))
		})
		It("cannot stat the master data directory", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, errors.New("permission denied")
			}
			fakeConns := []*services.Connection{}
			expectedErr := errors.Errorf("Error statting new directory %s_upgrade: permission denied", dir)
			err := hub.CreateAllDataDirectories(fakeConns, segDataDirMap)
			Expect(err.Error()).To(Equal(expectedErr.Error()))
		})
		It("cannot create the master data directory", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}
			utils.System.MkdirAll = func(path string, perm os.FileMode) error {
				return errors.New("permission denied")
			}
			fakeConns := []*services.Connection{}
			expectedErr := errors.New("Could not create new directory: permission denied")
			err := hub.CreateAllDataDirectories(fakeConns, segDataDirMap)
			Expect(err.Error()).To(Equal(expectedErr.Error()))
		})
		It("cannot create the segment data directories", func() {
			utils.System.Stat = func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}
			utils.System.MkdirAll = func(path string, perm os.FileMode) error {
				return nil
			}
			badConnection, _ := grpc.DialContext(context.Background(), "localhost:6416", grpc.WithInsecure())
			fakeConns := []*services.Connection{{badConnection, nil, "localhost", func() {}}}
			err := hub.CreateAllDataDirectories(fakeConns, segDataDirMap)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunInitsystemForNewCluster", func() {
		var (
			testExecutor *testhelper.TestExecutor
			stdout       *gbytes.Buffer
		)

		BeforeEach(func() {
			stdout, _, _ = testhelper.SetupTestLogger()
			testExecutor = &testhelper.TestExecutor{}
			source.Executor = testExecutor
		})
		It("successfully runs gpinitsystem", func() {
			testExecutor.LocalError = errors.New("exit status 1")
			err := hub.RunInitsystemForNewCluster("filepath")
			Expect(err).To(BeNil())
			testhelper.ExpectRegexp(stdout, "[WARNING]:-gpinitsystem completed with warnings")
		})
		It("runs gpinitsystem and fails", func() {
			testExecutor.LocalError = errors.New("exit status 2")
			testExecutor.LocalOutput = "some output"
			err := hub.RunInitsystemForNewCluster("filepath")
			Expect(err.Error()).To(Equal("gpinitsystem failed: some output: exit status 2"))
		})
		It("runs gpinitsystem and receives an interrupt", func() {
			testExecutor.LocalError = errors.New("exit status 127")
			testExecutor.LocalOutput = "some output"
			err := hub.RunInitsystemForNewCluster("filepath")
			Expect(err.Error()).To(Equal("gpinitsystem failed: some output: exit status 127"))
		})
	})

	Describe("GetMasterSegPrefix", func() {
		DescribeTable("returns a valid seg prefix given",
			func(datadir string) {
				segPrefix, err := services.GetMasterSegPrefix(datadir)
				Expect(segPrefix).To(Equal("gpseg"))
				Expect(err).ShouldNot(HaveOccurred())
			},
			Entry("an absolute path", "/data/master/gpseg-1"),
			Entry("a relative path", "../master/gpseg-1"),
			Entry("a implicitly relative path", "gpseg-1"),
		)

		DescribeTable("returns errors when given",
			func(datadir string) {
				_, err := services.GetMasterSegPrefix(datadir)
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
