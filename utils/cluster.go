package utils

import (
	"encoding/json"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gp-common-go-libs/operating"
	"github.com/pkg/errors"
)

const (
	SOURCE_CONFIG_FILENAME = "source_cluster_config.json"
	TARGET_CONFIG_FILENAME = "target_cluster_config.json"
)

type Cluster struct {
	*cluster.Cluster
	BinDir     string
	ConfigPath string
}

func NewMasterOnlyCluster(port int, host string, binDir string, configPath string) *Cluster {
	master := cluster.SegConfig{
		DbID:      1,
		ContentID: -1,
		Port:      port,
		Hostname:  host,
	}

	cc := cluster.Cluster{Segments: map[int]cluster.SegConfig{-1: master}}
	c := &Cluster{Cluster: &cc, BinDir: binDir, ConfigPath: configPath}
	return c
}

/*
 * We need to use an intermediary struct for reading and writing fields not
 * present in cluster.Cluster
 */
type ClusterConfig struct {
	SegConfigs []cluster.SegConfig
	BinDir     string
}

func (c *Cluster) Load() error {
	contents, err := System.ReadFile(c.ConfigPath)
	if err != nil {
		return err
	}
	clusterConfig := &ClusterConfig{}
	err = json.Unmarshal([]byte(contents), clusterConfig)
	if err != nil {
		return err
	}
	c.Cluster = cluster.NewCluster(clusterConfig.SegConfigs)
	c.BinDir = clusterConfig.BinDir
	return nil
}

func (c *Cluster) Commit() error {
	segConfigs := make([]cluster.SegConfig, 0)
	clusterConfig := &ClusterConfig{BinDir: c.BinDir}

	for _, contentID := range c.Cluster.ContentIDs {
		segConfigs = append(segConfigs, c.Segments[contentID])
	}

	clusterConfig.SegConfigs = segConfigs

	return WriteJSONFile(c.ConfigPath, clusterConfig)
}

func (c *Cluster) MasterDataDir() string {
	return c.GetDirForContent(-1)
}

func (c *Cluster) MasterHost() string {
	return c.GetHostForContent(-1)
}

func (c *Cluster) MasterPort() int {
	return c.GetPortForContent(-1)
}

func (c *Cluster) GetHostnames() []string {
	hostnameMap := make(map[string]bool, 0)
	for _, seg := range c.Segments {
		hostnameMap[seg.Hostname] = true
	}
	hostnames := make([]string, 0)
	for host := range hostnameMap {
		hostnames = append(hostnames, host)
	}
	return hostnames
}

func (c *Cluster) PrimaryHostnames() []string {
	hostnames := make(map[string]bool, 0)
	for _, seg := range c.Segments {
		// Ignore the master.
		if seg.ContentID >= 0 {
			hostnames[seg.Hostname] = true
		}
	}

	var list []string
	for host := range hostnames {
		list = append(list, host)
	}

	return list
}

// SegmentsOn returns the configurations of segments that are running on a given
// host. An error will be returned for unknown hostnames.
func (c Cluster) SegmentsOn(hostname string) ([]cluster.SegConfig, error) {
	var segments []cluster.SegConfig
	for _, segment := range c.Segments {
		if segment.Hostname == hostname {
			segments = append(segments, segment)
		}
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("cluster has no segments on host '%s'", hostname)
	}

	return segments, nil
}

// ExecuteOnAllHosts is a convenience wrapper for
// Cluster.GenerateAndExecuteCommand(..., ON_HOSTS_AND_MASTER). It will error
// out if the cluster doesn't have any loaded segments yet.
func (c *Cluster) ExecuteOnAllHosts(desc string, cmd func(contentID int) string) (*cluster.RemoteOutput, error) {
	if len(c.Segments) == 0 {
		return nil, errors.New("cluster has no loaded segments")
	}

	remoteOutput := c.GenerateAndExecuteCommand(desc, cmd, cluster.ON_HOSTS_AND_MASTER)
	return remoteOutput, nil
}

func (c *Cluster) NewDBConn() *dbconn.DBConn {
	defaultUser := "gpadmin"

	username := operating.System.Getenv("PGUSER")
	if username == "" {
		currentUser, err := operating.System.CurrentUser()
		if err != nil {
			gplog.Verbose("Error retrieving current os user, defaulting to %s", defaultUser)
			username = defaultUser
		} else {
			username = currentUser.Username
		}
	}

	return &dbconn.DBConn{
		ConnPool: nil,
		NumConns: 0,
		Driver:   dbconn.GPDBDriver{},
		User:     username,
		DBName:   "postgres",
		Host:     c.MasterHost(),
		Port:     c.MasterPort(),
		Tx:       nil,
		Version:  dbconn.GPDBVersion{},
	}
}

func (c *Cluster) RefreshConfig(dbConnector *dbconn.DBConn) error {
	err := dbConnector.Connect(1)
	if err != nil {
		return DatabaseConnectionError{Parent: err}
	}
	defer dbConnector.Close()

	dbConnector.Version.Initialize(dbConnector)
	segConfigs, err := cluster.GetSegmentConfiguration(dbConnector)
	if err != nil {
		return errors.Wrap(err, "Unable to get segment configuration for cluster")
	}

	c.Cluster = cluster.NewCluster(segConfigs)
	return nil
}
