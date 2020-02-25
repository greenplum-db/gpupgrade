package utils

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
)

type Cluster struct {
	// ContentIDs contains the list of all primary content IDs, in the same
	// order that they were provided to NewCluster. Clients requiring a stable
	// iteration order over the Primaries map may use this.
	ContentIDs []int

	// Primaries contains the primary SegConfigs, keyed by content ID. One
	// primary exists for every entry in ContentIDs.
	Primaries map[int]SegConfig

	// Mirrors contains any mirror SegConfigs, keyed by content ID. Not every
	// primary is guaranteed to have a corresponding mirror, so lookups should
	// check for key existence.
	Mirrors map[int]SegConfig

	BinDir  string
	Version dbconn.GPDBVersion
}

type SegConfig struct {
	DbID      int
	ContentID int
	Port      int
	Hostname  string
	DataDir   string
	Role      string
}

const (
	PrimaryRole = "p"
	MirrorRole  = "m"
)

// ClusterFromDB will create a Cluster by querying the passed DBConn for
// information. You must pass the cluster's binary directory, since it cannot be
// divined from the database.
func ClusterFromDB(conn *dbconn.DBConn, binDir string) (*Cluster, error) {
	err := conn.Connect(1)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't connect to cluster")
	}
	defer conn.Close()

	segments, err := GetSegmentConfiguration(conn)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't retrieve segment configuration")
	}

	c, err := NewCluster(segments)
	if err != nil {
		return nil, err
	}

	c.Version = conn.Version
	c.BinDir = binDir

	return c, nil
}

func (c *Cluster) MasterDataDir() string {
	return c.GetDirForContent(-1)
}

func (c *Cluster) MasterPort() int {
	return c.GetPortForContent(-1)
}

func (c *Cluster) MasterHostname() string {
	return c.Primaries[-1].Hostname
}

func (c *Cluster) HasStandby() bool {
	_, ok := c.Mirrors[-1]
	return ok
}

func (c *Cluster) StandbyPort() int {
	return c.Mirrors[-1].Port
}

func (c *Cluster) StandbyHostname() string {
	return c.Mirrors[-1].Hostname
}

func (c *Cluster) StandbyDataDirectory() string {
	return c.Mirrors[-1].DataDir
}

// XXX This does not provide mirror hostnames yet.
func (c *Cluster) GetHostnames() []string {
	hostnameMap := make(map[string]bool, 0)
	for _, seg := range c.Primaries {
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
	for _, seg := range c.Primaries {
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

// ErrUnknownHost can be returned by Cluster.SegmentsOn.
var ErrUnknownHost = xerrors.New("no such host in cluster")

type UnknownHostError struct {
	Hostname string
}

func (u UnknownHostError) Error() string {
	return fmt.Sprintf("cluster has no segments on host %q", u.Hostname)
}

func (u UnknownHostError) Is(err error) bool {
	return err == ErrUnknownHost
}

// SegmentsOn returns the configurations of segments that are running on a given
// host excluding the master. An error of type ErrUnknownHost will be returned
// for unknown hostnames.
func (c Cluster) SegmentsOn(hostname string) ([]SegConfig, error) {
	var segments []SegConfig
	for _, contentID := range c.ContentIDs {
		segment := c.Primaries[contentID]
		if segment.Hostname == hostname && segment.ContentID != -1 {
			segments = append(segments, segment)
		}
	}

	if len(segments) == 0 {
		return nil, UnknownHostError{hostname}
	}

	return segments, nil
}

// ErrInvalidSegments is returned by NewCluster if the segment configuration
// array does not map to a valid cluster.
var ErrInvalidSegments = errors.New("invalid segment configuration")

/*
 * Base cluster functions
 */

func NewCluster(segConfigs []SegConfig) (*Cluster, error) {
	cluster := Cluster{}

	cluster.Primaries = make(map[int]SegConfig)
	cluster.Mirrors = make(map[int]SegConfig)

	for _, seg := range segConfigs {
		content := seg.ContentID

		switch seg.Role {
		case PrimaryRole:
			// Check for duplication.
			if _, ok := cluster.Primaries[content]; ok {
				return nil, newInvalidSegmentsError(seg, "multiple primaries with content ID %d", content)
			}

			cluster.ContentIDs = append(cluster.ContentIDs, content)
			cluster.Primaries[content] = seg

		case MirrorRole:
			// Check for duplication.
			if _, ok := cluster.Mirrors[content]; ok {
				return nil, newInvalidSegmentsError(seg, "multiple mirrors with content ID %d", content)
			}

			cluster.Mirrors[content] = seg

		default:
			return nil, newInvalidSegmentsError(seg, "unknown role %q", seg.Role)
		}
	}

	// Make sure each mirror has a primary.
	for _, seg := range cluster.Mirrors {
		content := seg.ContentID

		if _, ok := cluster.Primaries[content]; !ok {
			return nil, newInvalidSegmentsError(seg, "mirror with content ID %d has no primary", content)
		}
	}

	return &cluster, nil
}

func (cluster *Cluster) GetContentList() []int {
	return cluster.ContentIDs
}

func (cluster *Cluster) GetDbidForContent(contentID int) int {
	return cluster.Primaries[contentID].DbID
}

func (cluster *Cluster) GetPortForContent(contentID int) int {
	return cluster.Primaries[contentID].Port
}

func (cluster *Cluster) GetHostForContent(contentID int) string {
	return cluster.Primaries[contentID].Hostname
}

func (cluster *Cluster) GetDirForContent(contentID int) string {
	return cluster.Primaries[contentID].DataDir
}

/*
 * Helper functions
 */

func GetSegmentConfiguration(connection *dbconn.DBConn) ([]SegConfig, error) {
	query := ""
	if connection.Version.Before("6") {
		query = `
SELECT
	s.dbid,
	s.content as contentid,
	s.port,
	s.hostname,
	e.fselocation as datadir,
	s.role
FROM gp_segment_configuration s
JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
JOIN pg_filespace f ON e.fsefsoid = f.oid
WHERE f.fsname = 'pg_system'
ORDER BY s.content;`
	} else {
		query = `
SELECT
	dbid,
	content as contentid,
	port,
	hostname,
	datadir,
	role
FROM gp_segment_configuration
ORDER BY content;`
	}

	results := make([]SegConfig, 0)
	err := connection.Select(&results, query)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func MustGetSegmentConfiguration(connection *dbconn.DBConn) []SegConfig {
	segConfigs, err := GetSegmentConfiguration(connection)
	gplog.FatalOnError(err)
	return segConfigs
}

// InvalidSegmentsError is the backing error type for ErrInvalidSegments. It
// contains the offending configuration object.
type InvalidSegmentsError struct {
	Segment SegConfig

	msg string
}

func newInvalidSegmentsError(seg SegConfig, format string, a ...interface{}) *InvalidSegmentsError {
	return &InvalidSegmentsError{
		Segment: seg,
		msg:     fmt.Sprintf(format, a...),
	}
}

func (i *InvalidSegmentsError) Error() string {
	return fmt.Sprintf("invalid segment configuration (%+v): %s", i.Segment, i.msg)
}

func (i *InvalidSegmentsError) Is(err error) bool {
	return err == ErrInvalidSegments
}
