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
	"github.com/greenplum-db/gpupgrade/utils/log"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/context"
)

func (h *Hub) UpgradeShareOids(ctx context.Context, in *idl.UpgradeShareOidsRequest) (*idl.UpgradeShareOidsReply, error) {
	gplog.Info("starting %s", upgradestatus.SHARE_OIDS)

	step, err := h.InitializeStep(upgradestatus.SHARE_OIDS)
	if err != nil {
		gplog.Error(err.Error())
		return &idl.UpgradeShareOidsReply{}, err
	}

	go func() {
		defer log.WritePanics()

		if err := h.shareOidFiles(); err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()

	return &idl.UpgradeShareOidsReply{}, nil
}

func (h *Hub) shareOidFiles() error {
	var err error
	rsyncFlags := "-rzpogt"

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
	for segmentID, segmentErr := range remoteOutput.Errors {
		if segmentErr != nil { // TODO: Refactor remoteOutput to return maps with keys and valid values, and not values that can be nil. If there is no value, then do not have a key.
			return multierror.Append(err, errors.Wrapf(segmentErr, "failed to copy master data directory to segment %d", segmentID))
		}
	}

	agentConns, connErr := h.AgentConns()
	if connErr != nil {
		return multierror.Append(err, connErr)
	}

	copyErr := CopyMasterDirectoryToSegmentDirectories(agentConns, h.target, destinationDirName)
	if copyErr != nil {
		return multierror.Append(err, copyErr)
	}

	return err
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
func contentsByHost(c *utils.Cluster, includeMaster bool) []int { // nolint: unparam
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
