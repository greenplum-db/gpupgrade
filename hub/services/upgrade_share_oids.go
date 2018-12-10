package services

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeShareOids(ctx context.Context, in *idl.UpgradeShareOidsRequest) (*idl.UpgradeShareOidsReply, error) {
	gplog.Info("Started processing share-oids request")

	go h.shareOidFiles()

	return &idl.UpgradeShareOidsReply{}, nil
}

func (h *Hub) shareOidFiles() {
	step := h.checklist.GetStepWriter(upgradestatus.SHARE_OIDS)

	err := step.ResetStateDir()
	if err != nil {
		gplog.Error("error from ResetStateDir " + err.Error())
		return
	}
	err = step.MarkInProgress()
	if err != nil {
		gplog.Error("error from MarkInProgress " + err.Error())
		return
	}

	anyFailed := false
	rsyncFlags := "-rzpogt"
	if h.source.Version.Before("6.0.0") {
		sourceDir := utils.MasterPGUpgradeDirectory(h.conf.StateDir)
		contents := contentsByHost(h.source, false)
		commandMap := make(map[int][]string, len(contents))

		for _, content := range contents {
			destinationDirectory := h.source.GetHostForContent(content) + ":" + utils.PGUpgradeDirectory(h.conf.StateDir)
			commandMap[content] = []string{"rsync", rsyncFlags, filepath.Join(sourceDir, "pg_upgrade_dump_*_oids.sql"), destinationDirectory}
		}

		remoteOutput := h.source.ExecuteClusterCommand(cluster.ON_HOSTS, commandMap)
		if remoteOutput.NumErrors > 0 {
			gplog.Error("Copying OID files failed with %d errors:", remoteOutput.NumErrors)
			for content, segmentErr := range remoteOutput.Errors {
				gplog.Error("Segment %d failed with error %s", content, segmentErr.Error())
			}
			anyFailed = true
		}
	} else {
		// Make sure sourceDir ends with a trailing slash so that rsync will
		// transfer the directory contents and not the directory itself.
		sourceDir := filepath.Clean(h.target.MasterDataDir()) + string(filepath.Separator)
		commandMap := make(map[int][]string, len(h.target.ContentIDs)-1)

		destinationDirName := "/tmp/masterDirCopy"

		/*
		 * Copy the directory once per host.
		 *
		 * We don't need to copy the master directory on the master host
		 * If there are primaries on the same host, the hostname will be
		 * added for the corresponding primaries.
		 */
		for _, content := range contentsByHost(h.target, false) {
			destinationDirectory := fmt.Sprintf("%s:%s", h.target.GetHostForContent(content), destinationDirName)
			commandMap[content] = []string{"rsync", rsyncFlags, sourceDir, destinationDirectory}
		}

		remoteOutput := h.source.ExecuteClusterCommand(cluster.ON_HOSTS, commandMap)
		if remoteOutput.NumErrors > 0 {
			gplog.Error("Copying master directory failed with %d errors:", remoteOutput.NumErrors)
			for content, segmentErr := range remoteOutput.Errors {
				gplog.Error("Segment %d failed with error %s", content, segmentErr.Error())
			}
			anyFailed = true
		}

		agentConns, err := h.AgentConns()
		err = CopyMasterDirectoryToSegmentDirectories(agentConns, h.target, destinationDirName)
		if err != nil {
			gplog.Error(err.Error())
			anyFailed = true
		}
	}

	if anyFailed {
		step.MarkFailed()
		if err != nil {
			gplog.Error("error from MarkFailed " + err.Error())
		}
	} else {
		step.MarkComplete()
		if err != nil {
			gplog.Error("error from MarkComplete " + err.Error())
		}
	}

}

func CopyMasterDirectoryToSegmentDirectories(agentConns []*Connection, target *utils.Cluster, destinationDirName string) error {
	segmentDataDirMap := map[string][]string{}
	for _, content := range target.ContentIDs {
		if content != -1 {
			segment := target.Segments[content]
			segmentDataDirMap[segment.Hostname] = append(segmentDataDirMap[segment.Hostname], target.GetDirForContent(content))
		}
	}

	errMsg := "Error copying master data directory to segment data directories"
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))
	for _, agentConn := range agentConns {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			client := idl.NewAgentClient(c.Conn)
			_, err := client.CopyMasterDirectoryToSegmentDirectories(context.Background(),
				&idl.CopyMasterDirRequest{
					MasterDir: destinationDirName,
					Datadirs:  segmentDataDirMap[c.Hostname],
				})

			if err != nil {
				gplog.Error("%s on host %s: %s", errMsg, c.Hostname, err.Error())
				errChan <- err
			}
		}(agentConn)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return errors.Wrap(err, errMsg)
		}
	}
	return nil
}

/*
 * Generate a list of content IDs such that running ExecuteClusterCommand
 * against them will execute once per host.
 */
func contentsByHost(c *utils.Cluster, includeMaster bool) []int {
	hostSegMap := make(map[string]int, 0)
	for content, seg := range c.Segments {
		if content == -1 && !includeMaster {
			continue
		}
		hostSegMap[seg.Hostname] = content
	}
	contents := []int{}
	for _, content := range hostSegMap {
		contents = append(contents, content)
	}
	return contents
}
