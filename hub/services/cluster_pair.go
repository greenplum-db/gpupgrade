package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/helpers"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type ClusterPair struct {
	OldCluster    *cluster.Cluster
	NewCluster    *cluster.Cluster
	OldBinDir     string
	NewBinDir     string
	CommandExecer helpers.CommandExecer
}

func (cp *ClusterPair) Init(baseDir, OldBinDir, NewBinDir string, execer helpers.CommandExecer) error {
	var err error
	cp.CommandExecer = execer

	err = cp.ReadOldConfig(baseDir)
	if err != nil {
		return fmt.Errorf("Couldn't read old config file: %+v", err)
	}
	err = cp.ReadNewConfig(baseDir)
	if err != nil {
		return fmt.Errorf("Couldn't read new config file: %+v", err)
	}
	cp.OldBinDir = OldBinDir
	cp.NewBinDir = NewBinDir
	return nil
}

func GetConfigFilePath(baseDir string) string {
	return filepath.Join(baseDir, "cluster_config.json")
}

func GetNewConfigFilePath(baseDir string) string {
	return filepath.Join(baseDir, "new_cluster_config.json")
}

/*
 * We need to use an intermediary struct for reading and writing fields not
 * present in cluster.Cluster
 */
type ClusterConfig struct {
	SegConfigs []cluster.SegConfig
	BinDir     string
}

func ReadClusterConfig(configFilePath string) (*cluster.Cluster, string, error) {
	contents, err := utils.System.ReadFile(configFilePath)
	if err != nil {
		return nil, "", err
	}
	clusterConfig := &ClusterConfig{}
	err = json.Unmarshal([]byte(contents), clusterConfig)
	if err != nil {
		return nil, "", err
	}
	return cluster.NewCluster(clusterConfig.SegConfigs), clusterConfig.BinDir, nil
}

func WriteClusterConfig(configFilePath string, c *cluster.Cluster, binDir string) error {
	segConfigs := make([]cluster.SegConfig, 0)
	clusterConfig := &ClusterConfig{BinDir: binDir}
	for _, contentID := range c.ContentIDs {
		segConfigs = append(segConfigs, c.Segments[contentID])
	}
	clusterConfig.SegConfigs = segConfigs
	contents, err := json.Marshal(clusterConfig)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to Marshal cluster config Json: %s", err.Error())
		return errors.New(errMsg)
	}
	// Write to a temporary file and move it over the old one, because WriteFile
	// will not truncate the original file, and it provides an atomic write
	tempFilePath := configFilePath + ".tmp"
	defer os.Remove(tempFilePath)
	err = utils.System.WriteFile(tempFilePath, contents, 0644)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to write temp config file: %s", err.Error())
		return errors.New(errMsg)
	}

	err = os.Rename(tempFilePath, configFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("Unable to Rename temp config file to \"cluster_config.json\": %s", err.Error())
		return errors.New(errMsg)
	}

	return err
}

func (cp *ClusterPair) ReadOldConfig(baseDir string) error {
	var err error
	cp.OldCluster, cp.OldBinDir, err = ReadClusterConfig(GetConfigFilePath(baseDir))
	return err
}

func (cp *ClusterPair) ReadNewConfig(baseDir string) error {
	var err error
	cp.NewCluster, cp.NewBinDir, err = ReadClusterConfig(GetNewConfigFilePath(baseDir))
	return err
}

func (cp *ClusterPair) WriteOldConfig(baseDir string) error {
	return WriteClusterConfig(GetConfigFilePath(baseDir), cp.OldCluster, cp.OldBinDir)
}

func (cp *ClusterPair) WriteNewConfig(baseDir string) error {
	return WriteClusterConfig(GetNewConfigFilePath(baseDir), cp.NewCluster, cp.NewBinDir)
}

func convert(b bool) string {
	if b {
		return "is"
	}
	return "is not"
}

func (cp *ClusterPair) StopEverything(pathToGpstopStateDir string, oldPostmasterRunning bool, newPostmasterRunning bool) {
	logmsg := "Shutting down clusters. The old cluster %s running. The new cluster %s running."
	gplog.Info(fmt.Sprintf(logmsg, convert(oldPostmasterRunning), convert(newPostmasterRunning)))
	checklistManager := upgradestatus.NewChecklistManager(pathToGpstopStateDir)

	if oldPostmasterRunning {
		cp.stopCluster(checklistManager, "gpstop.old", cp.OldBinDir, cp.OldCluster.GetDirForContent(-1))
	}

	if newPostmasterRunning {
		cp.stopCluster(checklistManager, "gpstop.new", cp.NewBinDir, cp.NewCluster.GetDirForContent(-1))
	}
}

func (cp *ClusterPair) EitherPostmasterRunning() (bool, bool) {
	oldPostmasterRunning := cp.IsPostmasterRunning(cp.OldCluster.GetDirForContent(-1))
	newPostmasterRunning := cp.IsPostmasterRunning(cp.NewCluster.GetDirForContent(-1))

	return oldPostmasterRunning, newPostmasterRunning
}

func (cp *ClusterPair) IsPostmasterRunning(masterDataDir string) bool {
	checkPidCmd := fmt.Sprintf("pgrep -F %s/postmaster.pid", masterDataDir)

	_, err := cp.OldCluster.ExecuteLocalCommand(checkPidCmd)
	if err != nil {
		gplog.Error("Could not determine whether the cluster with MASTER_DATA_DIRECTORY: %s is running: %+v",
			masterDataDir, err)
		return false
	}
	return true
}

func (cp *ClusterPair) stopCluster(stateManager *upgradestatus.ChecklistManager, step string, binDir string, masterDataDir string) {
	stateManager.ResetStateDir(step)
	err := stateManager.MarkInProgress(step)
	if err != nil {
		gplog.Error(err.Error())
		return
	}

	gpstopShellArgs := fmt.Sprintf("source %s/../greenplum_path.sh; %s/gpstop -a -d %s",
		binDir, binDir, masterDataDir)
	gplog.Info("gpstop args: %+v", gpstopShellArgs)
	_, err = cp.OldCluster.ExecuteLocalCommand(gpstopShellArgs)

	gplog.Info("finished stopping %s", step)
	if err != nil {
		gplog.Error(err.Error())
		stateManager.MarkFailed(step)
		return
	}

	stateManager.MarkComplete(step)
}

func (cp *ClusterPair) GetPortsAndDataDirForReconfiguration() (int, int, string) {
	return cp.OldCluster.GetPortForContent(-1), cp.NewCluster.GetPortForContent(-1), cp.NewCluster.GetDirForContent(-1)
}

func (cp *ClusterPair) GetMasterPorts() (int, int) {
	return cp.OldCluster.GetPortForContent(-1), cp.NewCluster.GetPortForContent(-1)
}

func (cp *ClusterPair) GetMasterDataDirs() (string, string) {
	return cp.OldCluster.GetDirForContent(-1), cp.NewCluster.GetDirForContent(-1)
}

func (cp *ClusterPair) GetHostnames() []string {
	hostnameMap := make(map[string]bool, 0)
	for _, seg := range cp.OldCluster.Segments {
		hostnameMap[seg.Hostname] = true
	}
	hostnames := make([]string, 0)
	for host := range hostnameMap {
		hostnames = append(hostnames, host)
	}
	return hostnames
}
