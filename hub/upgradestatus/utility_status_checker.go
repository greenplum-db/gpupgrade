package upgradestatus

import (
	"path/filepath"
	"strings"
	"time"

	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"os"

	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gp-common-go-libs/iohelper"
)

/*
 assumptions here are:
	- pg_upgrade will not fail without error before writing an inprogress file
	- when a new pg_upgrade is started it deletes all *.done and *.inprogress files
*/
func SegmentConversionStatus(pgUpgradePath, oldDataDir string, executor cluster.Executor) pb.StepStatus {
	return GetUtilityStatus("pg_upgrade", pgUpgradePath, oldDataDir, "*.inprogress", executor, isUpgradeComplete)
}

func GetUtilityStatus(binaryName, utilityStatePath, dataDir, progressFilePattern string, executor cluster.Executor, isCompleteFunc func(string) bool) pb.StepStatus {
	_, err := utils.System.Stat(utilityStatePath)
	switch {
	case utils.System.IsNotExist(err):
		return pb.StepStatus_PENDING
	case isBinaryRunning(dataDir, executor):
		return pb.StepStatus_RUNNING
	case !inProgressFilesExist(utilityStatePath, progressFilePattern) && isCompleteFunc(utilityStatePath):
		return pb.StepStatus_COMPLETE
	default:
		return pb.StepStatus_FAILED
	}
}

/*
 * We call external binaries with an external data directory, so passing in the
 * data directory allows finding that particular invocation if other processes
 * using the same binary are running concurrently.
 */
func isBinaryRunning(dataDir string, executor cluster.Executor) bool {
	command := "ps -ef | grep [p]g_upgrade"
	if dataDir != "" {
		command = fmt.Sprintf("%s | grep %s", command, dataDir)
	}
	binaryPids, err := executor.ExecuteLocalCommand(command)
	if err == nil && len(binaryPids) != 0 {
		return true
	}
	return false
}

// We need progressFilePattern because pg_upgrade uses dir/*.inprogress, while gpstop uses dir/*/in.progress
func inProgressFilesExist(utilityStatePath, progressFilePattern string) bool {
	files, err := utils.System.FilePathGlob(filepath.Join(utilityStatePath, progressFilePattern))
	if files == nil {
		return false
	}

	if err != nil {
		gplog.Error("Error determining step status: ", err)
		return false
	}

	return true
}

func isUpgradeComplete(pgUpgradePath string) bool {
	doneFiles, doneErr := utils.System.FilePathGlob(pgUpgradePath + "/*.done")
	if doneFiles == nil {
		return false
	}
	if doneErr != nil {
		gplog.Error(doneErr.Error())
		return false
	}

	var latestDoneFile string
	var latestDoneFileModTime time.Time
	for _, doneFile := range doneFiles {
		fi, err := os.Stat(doneFile)
		if err != nil {
			gplog.Error("Done file %v cannot be read", doneFile)
			continue
		}
		if fi.ModTime().After(latestDoneFileModTime) {
			latestDoneFile = doneFile
			latestDoneFileModTime = fi.ModTime()
		}
	}

	contents, err := iohelper.ReadLinesFromFile(latestDoneFile)
	if err != nil {
		gplog.Error("Error reading done file: %v", err)
		return false
	}
	for _, line := range contents {
		if strings.Contains(line, "Upgrade complete") {
			return true
		}
	}
	return false
}

func isStopComplete(gpstopStatePath string) bool {
	completeFiles, completeErr := utils.System.FilePathGlob(gpstopStatePath + "/*/completed")
	if completeFiles == nil {
		return false
	}

	if completeErr != nil {
		gplog.Error(completeErr.Error())
		return false
	}

	/* There should only be two completed files.
	 * One for gpstop.old and one for gpstop.new
	 */
	if len(completeFiles) == 2 {
		return true
	}

	return false
}
