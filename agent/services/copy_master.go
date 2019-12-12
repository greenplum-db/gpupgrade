package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *AgentServer) CopyMaster(ctx context.Context, in *idl.CopyMasterRequest) (*idl.CopyMasterReply, error) {
	gplog.Info("got a request to copy the master data directory to the segment hosts from the hub")

	masterDir := in.MasterDir
	datadirs := in.Datadirs
	var err error
	for _, segDataDir := range datadirs {
		err = checkSegDirExists(segDataDir)
		if err != nil {
			break
		}
		err = backupSegDir(segDataDir)
		if err != nil {
			break
		}
		err = copyMasterDirOverSegment(s.executor, masterDir, segDataDir)
		if err != nil {
			break
		}
		err = restoreSegmentFiles(segDataDir)
		if err != nil {
			break
		}
		err = removeMasterFilesFromSegment(segDataDir)
		if err != nil {
			break
		}
	}
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CopyMasterReply{}, err
	}

	err = removeMasterDir(masterDir)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.CopyMasterReply{}, err
	}

	return &idl.CopyMasterReply{}, err
}

func checkSegDirExists(segDataDir string) error {
	_, err := utils.System.Stat(segDataDir)
	if os.IsNotExist(err) {
		return errors.Wrapf(err, "Segment data directory %s does not exist", segDataDir)
	} else if err != nil {
		return errors.Wrapf(err, "Error statting segment data directory %s", segDataDir)
	}
	return nil
}

func backupSegDir(segDataDir string) error {
	backupSegDir := fmt.Sprintf("%s.old", segDataDir)
	err := utils.System.Rename(segDataDir, backupSegDir)
	if err != nil {
		return errors.Wrapf(err, "Could not back up segment data directory")
	}
	return nil
}

// Overwrite the segment directory with the rsync'd master directory
func copyMasterDirOverSegment(executor cluster.Executor, masterDir string, segDataDir string) error {
	output, err := executor.ExecuteLocalCommand(fmt.Sprintf("cp -a %s %s", masterDir, segDataDir))
	if err != nil {
		return errors.Wrapf(err, "Could not copy master data directory to segment data directory: %s", output)
	}
	return nil
}

// Put segment config files back, overwriting the master versions
func restoreSegmentFiles(segDataDir string) error {
	// Files that differ between the master and the segments, where we want to keep the segment versions
	filesToPreserve := []string{
		"internal.auto.conf",
		"postgresql.conf",
		"pg_hba.conf",
		"postmaster.opts",
	}
	backupSegDir := fmt.Sprintf("%s.old", segDataDir)
	for _, file := range filesToPreserve {
		err := utils.System.Rename(filepath.Join(backupSegDir, file), filepath.Join(segDataDir, file))
		if err != nil {
			return errors.Wrapf(err, "Could not copy %s from backup segment directory to segment data directory", file)
		}
	}
	return nil
}

// Remove remaining master-specific files and directories
func removeMasterFilesFromSegment(segDataDir string) error {
	// Files that are present on the master but the not the segments, where we want to delete them from the segments
	// gppperfmon is a directory, so we'll call RemoveAll on these.
	filesToRemove := []string{"gp_dbid", "gpssh.conf", "gpperfmon"}
	for _, file := range filesToRemove {
		err := utils.System.RemoveAll(filepath.Join(segDataDir, file))
		if err != nil {
			return errors.Wrapf(err, "Could not remove %s from segment data directory", file)
		}
	}
	return nil
}

// Remove the copied master data directory from the segment hosts
func removeMasterDir(masterDir string) error {
	err := utils.System.RemoveAll(masterDir)
	if err != nil {
		return errors.Wrapf(err, "Could not delete copy of master data directory")
	}
	return nil
}
