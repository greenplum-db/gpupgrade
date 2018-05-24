package testutils

import (
	"errors"
	"github.com/greenplum-db/gpupgrade/hub/configutils"
)

type SpyConfigReader struct {
	FailToGetHostnames bool
	HostnamesListEmpty bool
}

func NewSpyConfigReader() *SpyConfigReader { return &SpyConfigReader{} }

func (scr *SpyConfigReader) GetHostnames() ([]string, error) {
	if scr.FailToGetHostnames {
		return nil, errors.New("force failure - no config")
	}
	if scr.HostnamesListEmpty == true {
		return []string{}, nil
	} else {
		return []string{"somehost"}, nil
	}
}

func (scr *SpyConfigReader) GetSegmentConfiguration() configutils.SegmentConfiguration {return nil}
func (scr *SpyConfigReader) OfOldClusterConfig(baseDir string) {}
func (scr *SpyConfigReader) OfNewClusterConfig(baseDir string) {}
func (scr *SpyConfigReader) GetMasterDataDir() string {return ""}
func (scr *SpyConfigReader) GetPortForSegment(segmentDbid int) int {return 0}
