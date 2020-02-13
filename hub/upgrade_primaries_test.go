package hub_test

import (
	"errors"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetDataDirPairs", func() {
	It("returns an error if new config does not contain all the same content as the old config", func() {
		newTarget, err := utils.NewCluster([]utils.SegConfig{
			{ContentID: 0, Hostname: "localhost", DataDir: "new/datadir1", Port: 11, Role: utils.PrimaryRole},
		})
		Expect(err).NotTo(HaveOccurred())
		testHub.Target = newTarget

		_, err = testHub.GetDataDirPairs()

		Expect(err).To(HaveOccurred())
		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})

	It("returns an error if the content matches, but the hostname does not", func() {
		differentSeg := target.Primaries[0]
		differentSeg.Hostname = "localhost2"
		target.Primaries[0] = differentSeg

		_, err := testHub.GetDataDirPairs()

		Expect(err).To(HaveOccurred())
		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})
})

var _ = Describe("UpgradePrimaries", func() {
	It("returns nil error, and agent receives only expected segmentConfig values", func() {
		seg1 := target.Primaries[0]
		seg1.DataDir = filepath.Join(dir, "seg1_upgrade")
		seg1.Port = 27432
		target.Primaries[0] = seg1

		seg2 := target.Primaries[1]
		seg2.DataDir = filepath.Join(dir, "seg2_upgrade")
		seg2.Port = 27433

		// Set up both segments to be on the same host (but still distinct from
		// the master host).
		seg2.Hostname = seg1.Hostname
		target.Primaries[1] = seg2

		// Source hostnames must match the target.
		sourceSeg2 := source.Primaries[1]
		sourceSeg2.Hostname = seg2.Hostname
		source.Primaries[1] = sourceSeg2

		agentConns, _ := testHub.AgentConns()
		dataDirPairMap, _ := testHub.GetDataDirPairs()

		err := hub.UpgradePrimaries(false, "/some/cool/backupdir", agentConns, dataDirPairMap, source, target, useLinkMode)
		Expect(err).ToNot(HaveOccurred())

		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.SourceBinDir).To(Equal("/source/bindir"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.TargetBinDir).To(Equal("/target/bindir"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.MasterBackupDir).To(Equal("/some/cool/backupdir"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.DataDirPairs).To(ConsistOf([]*idl.DataDirPair{
			{
				SourceDataDir: filepath.Join(dir, "seg1"),
				TargetDataDir: filepath.Join(dir, "seg1_upgrade"),
				Content:       0,
				SourcePort:    25432,
				TargetPort:    27432,
				DBID:          2,
			},
			{
				SourceDataDir: filepath.Join(dir, "seg2"),
				TargetDataDir: filepath.Join(dir, "seg2_upgrade"),
				Content:       1,
				SourcePort:    25433,
				TargetPort:    27433,
				DBID:          3,
			},
		}))
	})

	It("returns an error if any upgrade primary call to any agent fails", func() {
		mockAgent.Err <- errors.New("fail upgrade primary call")

		agentConns, _ := testHub.AgentConns()
		dataDirPairMap, _ := testHub.GetDataDirPairs()

		err := hub.UpgradePrimaries(false, "", agentConns, dataDirPairMap, source, target, useLinkMode)
		Expect(err).To(HaveOccurred())

		Expect(mockAgent.NumberOfCalls()).To(Equal(2))
	})
})
