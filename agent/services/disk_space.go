package services

import (
	"context"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/cloudfoundry/gosigar"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

func (s *AgentServer) CheckDiskSpaceOnAgents(ctx context.Context, in *idl.CheckDiskSpaceRequestToAgent) (*idl.CheckDiskSpaceReplyFromAgent, error) {
	gplog.Info("got a check disk command from the hub")
	diskUsage, err := s.GetDiskUsage()
	if err != nil {
		gplog.Error(err.Error())
		return nil, err
	}
	var listDiskUsages []*idl.FileSysUsage
	for k, v := range diskUsage {
		listDiskUsages = append(listDiskUsages, &idl.FileSysUsage{Filesystem: k, Usage: v})
	}
	return &idl.CheckDiskSpaceReplyFromAgent{ListOfFileSysUsage: listDiskUsages}, nil
}

// diskUsage() wraps a pair of calls to the gosigar library.
// This is local repetition of the sys_utils function pointer pattern. If there was more than one of these,
// we would've refactored.
// "Adapted" from the gosigar usage example at https://github.com/cloudfoundry/gosigar/blob/master/examples/df.go
func diskUsage() (map[string]float64, error) {
	diskUsagePerFS := make(map[string]float64)
	fslist := sigar.FileSystemList{}
	err := fslist.Get()
	if err != nil {
		gplog.Error(err.Error())
		return nil, err
	}

	for _, fs := range fslist.List {
		dirName := fs.DirName

		usage := sigar.FileSystemUsage{}

		err = usage.Get(dirName)
		if err != nil {
			gplog.Error(err.Error())
			return nil, err
		}

		diskUsagePerFS[dirName] = usage.UsePercent()
	}
	return diskUsagePerFS, nil
}
