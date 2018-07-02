package upgradestatus

import (
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"bufio"
	"io"
	"os"
	"regexp"

	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

// Type of segment being upgraded
type SegmentType int

const (
	MASTER SegmentType = iota
	PRIMARY
)

type ConvertSegment struct {
	segType       SegmentType
	pgUpgradePath string
	oldDataDir    string
	executor      cluster.Executor
}

func NewPGUpgradeStatusChecker(segType SegmentType, pgUpgradePath, oldDataDir string, executor cluster.Executor) ConvertSegment {
	return ConvertSegment{
		segType:       segType,
		pgUpgradePath: pgUpgradePath,
		oldDataDir:    oldDataDir,
		executor:      executor,
	}
}

/*
 assumptions here are:
	- pg_upgrade will not fail without error before writing an inprogress file
	- when a new pg_upgrade is started it deletes all *.done and *.inprogress files
*/
func (c *ConvertSegment) GetStatus() *pb.UpgradeStepStatus {
	//make default reply object
	upgradeStatus := &pb.UpgradeStepStatus{
		Status: pb.StepStatus_FAILED,
	}
	//update Step in reply object to match the conversion
	if c.segType == MASTER {
		upgradeStatus.Step = pb.UpgradeSteps_MASTERUPGRADE
	} else if c.segType == PRIMARY {
		upgradeStatus.Step = pb.UpgradeSteps_CONVERT_PRIMARIES
	}
	pgUpgradePath := c.pgUpgradePath

	//update Status in reply object
	if _, err := utils.System.Stat(pgUpgradePath); utils.System.IsNotExist(err) {
		upgradeStatus.Status = pb.StepStatus_PENDING
		return upgradeStatus
	}

	if c.pgUpgradeRunning() {
		upgradeStatus.Status = pb.StepStatus_RUNNING
		return upgradeStatus
	}

	if !inProgressFilesExist(pgUpgradePath) && c.IsUpgradeComplete(pgUpgradePath) {
		upgradeStatus.Status = pb.StepStatus_COMPLETE
		return upgradeStatus
	}

	//return default
	return upgradeStatus
}

func (c *ConvertSegment) pgUpgradeRunning() bool {
	//if pgrep doesnt find target, ExecCmdOutput will return empty byte array and err.Error()="exit status 1"
	pgUpgradePids, err := c.executor.ExecuteLocalCommand(fmt.Sprintf("pgrep pg_upgrade | grep --old-datadir=%s", c.oldDataDir))
	if err == nil && len(pgUpgradePids) != 0 {
		return true
	}
	return false
}

func inProgressFilesExist(pgUpgradePath string) bool {
	files, err := utils.System.FilePathGlob(pgUpgradePath + "/*.inprogress")
	if files == nil {
		return false
	}

	if err != nil {
		gplog.Error("err is: ", err)
		return false
	}

	return true
}

func (c ConvertSegment) IsUpgradeComplete(pgUpgradePath string) bool {
	doneFiles, doneErr := utils.System.FilePathGlob(pgUpgradePath + "/*.done")
	if doneFiles == nil {
		return false
	}

	if doneErr != nil {
		gplog.Error(doneErr.Error())
		return false
	}

	/* Get the latest done file
	 * Parse and find the "Upgrade complete" and return true.
	 * otherwise, return false.
	 */

	latestDoneFile := doneFiles[0]
	fi, err := utils.System.Stat(latestDoneFile)
	if err != nil {
		gplog.Error("IsUpgradeComplete: %v", err)
		return false
	}

	latestDoneFileModTime := fi.ModTime()
	for i := 1; i < len(doneFiles); i++ {
		doneFile := doneFiles[i]
		fi, err = os.Stat(doneFile)
		if err != nil {
			gplog.Error("Done file cannot be read: %v", doneFile)
			continue
		}

		if fi.ModTime().After(latestDoneFileModTime) {
			latestDoneFile = doneFiles[i]
			latestDoneFileModTime = fi.ModTime()
		}
	}

	f, err := utils.System.Open(latestDoneFile)
	if err != nil {
		gplog.Error(err.Error())
	}
	defer f.Close()
	r := bufio.NewReader(f)
	line, err := r.ReadString('\n')

	// It is possible for ReadString to return a valid line and
	// be EOF if the file has only 1 line
	re := regexp.MustCompile("Upgrade complete")
	for err != io.EOF {
		if err != nil {
			gplog.Error("IsUpgradeComplete: %v", err)
			return false
		}

		if re.FindString(line) != "" {
			return true
		}

		line, err = r.ReadString('\n')
	}

	return false
}
