package services

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// TODO: consolidate with RetrieveAndSaveOldConfig(); it's basically the same
// code
func SaveTargetClusterConfig(target *utils.Cluster, dbConnector *dbconn.DBConn, stateDir string) error {
	err := os.MkdirAll(stateDir, 0700)
	if err != nil {
		return err
	}

	segConfigs, err := cluster.GetSegmentConfiguration(dbConnector)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to get segment configuration for new cluster: %s", err.Error())
		return errors.New(errMsg)
	}
	target.Cluster = cluster.NewCluster(segConfigs)

	err = target.Commit()
	return err
}

func (h *Hub) PrepareInitCluster(ctx context.Context, in *pb.PrepareInitClusterRequest) (*pb.PrepareInitClusterReply, error) {
	gplog.Info("Running PrepareInitCluster()")
	dbConnector := db.NewDBConn("localhost", int(h.source.MasterPort()),
		"template1")

	go func() {
		step := h.checklist.GetStepWriter(upgradestatus.INIT_CLUSTER)
		err := h.InitCluster(dbConnector)
		if err != nil {
			gplog.Error(err.Error())
			step.MarkFailed()
		} else {
			step.MarkComplete()
		}
	}()
	return &pb.PrepareInitClusterReply{}, nil
}

func (h *Hub) InitCluster(dbConnector *dbconn.DBConn) error {
	defer dbConnector.Close()

	step := h.checklist.GetStepWriter(upgradestatus.INIT_CLUSTER)
	segmentDataDirMap := map[string][]string{}
	agentConns := []*Connection{}
	gpinitsystemFilepath := filepath.Join(h.conf.StateDir, "gpinitsystem_config")

	err := initializeState(step)
	if err != nil {
		return err
	}
	err = dbConnector.Connect(1)
	if err != nil {
		return errors.Wrap(err, "Could not connect to database")
	}
	dbConnector.Version.Initialize(dbConnector)

	gpinitsystemConfig, err := h.CreateInitialInitsystemConfig()
	if err != nil {
		return err
	}
	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, dbConnector)
	if err != nil {
		return err
	}
	agentConns, err = h.AgentConns()
	if err != nil {
		return errors.Wrap(err, "Could not get/create agents")
	}
	gpinitsystemConfig, segmentDataDirMap = h.DeclareDataDirectories(gpinitsystemConfig)
	err = h.CreateAllDataDirectories(agentConns, segmentDataDirMap)
	if err != nil {
		return err
	}
	err = WriteInitsystemFile(gpinitsystemConfig, gpinitsystemFilepath)
	if err != nil {
		return err
	}
	err = h.RunInitsystemForNewCluster(gpinitsystemFilepath)
	if err != nil {
		return err
	}
	err = SaveTargetClusterConfig(h.target, dbConnector, h.conf.StateDir)
	if err != nil {
		return errors.Wrap(err, "Could not save new cluster configuration")
	}
	return nil
}

func initializeState(step upgradestatus.StateWriter) error {
	err := step.ResetStateDir()
	if err != nil {
		return errors.Wrap(err, "Could not reset state dir")
	}

	err = step.MarkInProgress()
	if err != nil {
		return errors.Wrap(err, "Could not mark in progress")
	}
	return nil
}

func GetCheckpointSegmentsAndEncoding(gpinitsystemConfig []string, dbConnector *dbconn.DBConn) ([]string, error) {
	checkpointSegments, err := dbconn.SelectString(dbConnector, "SELECT current_setting('checkpoint_segments') AS string")
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not retrieve checkpoint segments")
	}
	encoding, err := dbconn.SelectString(dbConnector, "SELECT current_setting('server_encoding') AS string")
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not retrieve server encoding")
	}
	gpinitsystemConfig = append(gpinitsystemConfig,
		fmt.Sprintf("CHECK_POINT_SEGMENTS=%s", checkpointSegments),
		fmt.Sprintf("ENCODING=%s", encoding))
	return gpinitsystemConfig, nil
}

func (h *Hub) CreateInitialInitsystemConfig() ([]string, error) {
	gpinitsystemConfig := []string{`ARRAY_NAME="gp_upgrade cluster"`}

	//seg prefix
	sourceDataDir := h.source.MasterDataDir()

	segPrefix, err := GetMasterSegPrefix(sourceDataDir)
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not get master segment prefix")
	}

	gplog.Info("Data Dir: %s", sourceDataDir)
	gplog.Info("segPrefix: %v", segPrefix)
	gpinitsystemConfig = append(gpinitsystemConfig, "SEG_PREFIX="+segPrefix, "TRUSTED_SHELL=ssh")

	return gpinitsystemConfig, nil
}

func WriteInitsystemFile(gpinitsystemConfig []string, gpinitsystemFilepath string) error {
	gpinitsystemContents := []byte(strings.Join(gpinitsystemConfig, "\n"))

	err := ioutil.WriteFile(gpinitsystemFilepath, gpinitsystemContents, 0644)
	if err != nil {
		return errors.Wrap(err, "Could not write gpinitsystem_config file")
	}
	return nil
}

func (h *Hub) DeclareDataDirectories(gpinitsystemConfig []string) ([]string, map[string][]string) {
	// declare master data directory
	master := h.source.Segments[-1]
	master.Port += 1
	master.DataDir = fmt.Sprintf("%s_upgrade/%s", path.Dir(master.DataDir), path.Base(master.DataDir))
	datadirDeclare := fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%d~%s~%d~%d~0",
		master.Hostname, master.Port, master.DataDir, master.DbID, master.ContentID)
	gpinitsystemConfig = append(gpinitsystemConfig, datadirDeclare)
	// declare segment data directories
	segmentDataDirMap := map[string][]string{}
	segmentDeclarations := []string{}
	for _, content := range h.source.ContentIDs {
		if content != -1 {
			segment := h.source.Segments[content]
			// FIXME: Arbitrary assumption.	 Do something smarter later
			segment.Port += 2000
			datadir := fmt.Sprintf("%s_upgrade", path.Dir(segment.DataDir))
			segment.DataDir = fmt.Sprintf("%s/%s", datadir, path.Base(segment.DataDir))
			segmentDataDirMap[segment.Hostname] = append(segmentDataDirMap[segment.Hostname],
				datadir)
			declaration := fmt.Sprintf("\t%s~%d~%s~%d~%d~0",
				segment.Hostname, segment.Port, segment.DataDir, segment.DbID, segment.ContentID)
			segmentDeclarations = append(segmentDeclarations, declaration)
		}
	}
	datadirDeclare = fmt.Sprintf("declare -a PRIMARY_ARRAY=(\n%s\n)", strings.Join(segmentDeclarations, "\n"))
	gpinitsystemConfig = append(gpinitsystemConfig, datadirDeclare)
	return gpinitsystemConfig, segmentDataDirMap
}

func (h *Hub) CreateAllDataDirectories(agentConns []*Connection, segmentDataDirMap map[string][]string) error {
	// create master data directory for gpinitsystem if it doesn't exist
	targetDataDir := path.Dir(h.source.MasterDataDir()) + "_upgrade"
	_, err := utils.System.Stat(targetDataDir)
	if os.IsNotExist(err) {
		err = utils.System.MkdirAll(targetDataDir, 0755)
		if err != nil {
			return errors.Wrap(err, "Could not create new directory")
		}
	} else if err != nil {
		return errors.Wrapf(err, "Error statting new directory %s", targetDataDir)
	}
	// create segment data directories for gpinitsystem if they don't exist
	err = CreateSegmentDataDirectories(agentConns, segmentDataDirMap)
	if err != nil {
		return errors.Wrap(err, "Could not create segment data directories")
	}
	return nil
}

func (h *Hub) RunInitsystemForNewCluster(gpinitsystemFilepath string) error {
	// gpinitsystem the new cluster
	cmdStr := fmt.Sprintf("gpinitsystem -a -I %s", gpinitsystemFilepath)
	output, err := h.source.Executor.ExecuteLocalCommand(cmdStr)
	if err != nil {
		// gpinitsystem has a return code of 1 for warnings, so we can ignore that return code
		if err.Error() == "exit status 1" {
			gplog.Warn("gpinitsystem completed with warnings")
			return nil
		}
		return errors.Wrapf(err, "gpinitsystem failed: %s", output)
	}
	return nil
}

func GetMasterSegPrefix(datadir string) (string, error) {
	const masterContentID = "-1"

	base := path.Base(datadir)
	if !strings.HasSuffix(base, masterContentID) {
		return "", fmt.Errorf("path requires a master content identifier: '%s'", datadir)
	}

	segPrefix := strings.TrimSuffix(base, masterContentID)
	if segPrefix == "" {
		return "", fmt.Errorf("path has no segment prefix: '%s'", datadir)
	}
	return segPrefix, nil
}

func CreateSegmentDataDirectories(agentConns []*Connection, dataDirMap map[string][]string) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))
	for _, agentConn := range agentConns {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			client := pb.NewAgentClient(c.Conn)
			_, err := client.CreateSegmentDataDirectories(context.Background(),
				&pb.CreateSegmentDataDirRequest{
					Datadirs: dataDirMap[agentConn.Hostname],
				})

			if err != nil {
				gplog.Error("Error creating segment data directories on host %s: %s",
					agentConn.Hostname, err.Error())
				errChan <- err
			}
		}(agentConn)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return errors.Wrap(err, "Error creating segment data directories")
		}
	}
	return nil
}
