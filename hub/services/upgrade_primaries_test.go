package services_test

import (
	"errors"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"

	"github.com/greenplum-db/gpupgrade/idl"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("hub.ExecuteUpgradePrimariesSubStep()", func() {
	It("returns nil error, and agent receives only expected segmentConfig values", func() {
		seg1 := target.Segments[0]
		seg1.DataDir = filepath.Join(dir, "seg1_upgrade")
		seg1.Port = 27432
		target.Segments[0] = seg1

		seg2 := target.Segments[1]
		seg2.DataDir = filepath.Join(dir, "seg2_upgrade")
		seg2.Port = 27433

		// Set up both segments to be on the same host (but still distinct from
		// the master host).
		seg2.Hostname = seg1.Hostname
		target.Segments[1] = seg2

		// Source hostnames must match the target.
		sourceSeg2 := source.Segments[1]
		sourceSeg2.Hostname = seg2.Hostname
		source.Segments[1] = sourceSeg2

		err := hub.ConvertPrimaries(false)
		Expect(err).ToNot(HaveOccurred())

		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.OldBinDir).To(Equal("/source/bindir"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.NewBinDir).To(Equal("/target/bindir"))
		Expect(mockAgent.UpgradeConvertPrimarySegmentsRequest.DataDirPairs).To(ConsistOf([]*idl.DataDirPair{
			{OldDataDir: filepath.Join(dir, "seg1"), NewDataDir: filepath.Join(dir, "seg1_upgrade"), Content: 0, OldPort: 25432, NewPort: 27432, DBID: 2},
			{OldDataDir: filepath.Join(dir, "seg2"), NewDataDir: filepath.Join(dir, "seg2_upgrade"), Content: 1, OldPort: 25433, NewPort: 27433, DBID: 3},
		}))
	})

	It("returns an error if new config does not contain all the same content as the old config", func() {
		target.Cluster = &cluster.Cluster{
			ContentIDs: []int{0},
			Segments: map[int]cluster.SegConfig{
				0: newSegment(0, "localhost", "new/datadir1", 11),
			},
		}

		err := hub.ConvertPrimaries(false)
		Expect(err).To(HaveOccurred())
		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})

	It("returns an error if the content matches, but the hostname does not", func() {
		differentSeg := target.Segments[0]
		differentSeg.Hostname = "localhost2"
		target.Segments[0] = differentSeg

		err := hub.ConvertPrimaries(false)
		Expect(err).To(HaveOccurred())

		Expect(mockAgent.NumberOfCalls()).To(Equal(0))
	})

	It("returns an error if any upgrade primary call to any agent fails", func() {
		mockAgent.Err <- errors.New("fail upgrade primary call")

		err := hub.ConvertPrimaries(false)
		Expect(err).To(HaveOccurred())

		Expect(mockAgent.NumberOfCalls()).To(Equal(2))
	})
})

func newSegment(content int, hostname, dataDir string, port int) cluster.SegConfig {
	return cluster.SegConfig{
		ContentID: content,
		Hostname:  hostname,
		DataDir:   dataDir,
		Port:      port,
	}
}
