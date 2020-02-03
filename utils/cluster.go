package utils

import (
	"encoding/json"
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
)

type Cluster struct {
	*cluster.Cluster

	BinDir  string
	Version dbconn.GPDBVersion
}

// ClusterFromDB will create a Cluster by querying the passed DBConn for
// information. You must pass the cluster's binary directory, since it cannot be
// divined from the database.
func ClusterFromDB(conn *dbconn.DBConn, binDir string) (*Cluster, error) {
	err := conn.Connect(1)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't connect to cluster")
	}
	defer conn.Close()

	c := new(Cluster)
	c.Version = conn.Version

	segments, err := cluster.GetSegmentConfiguration(conn)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't retrieve segment configuration")
	}

	c.Cluster = cluster.NewCluster(segments)
	c.BinDir = binDir

	return c, nil
}

func (c *Cluster) MasterDataDir() string {
	return c.GetDirForContent(-1)
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

// serializableCluster contains all of the members of utils.Cluster that can be
// serialized to disk.
//
// Ideally, utils.Cluster would be serializable itself, but unfortunately the
// Executor member cannot be put through a JSON marshal/unmarshal round trip. We
// exclude it here.
type serializableCluster struct {
	ContentIDs []int
	Segments   map[int]cluster.SegConfig
	BinDir     string
	Version    dbconn.GPDBVersion
}

func newSerializableCluster(c *Cluster) *serializableCluster {
	return &serializableCluster{
		c.ContentIDs,
		c.Segments,
		c.BinDir,
		c.Version,
	}
}

func (s *serializableCluster) cluster() *Cluster {
	// Members are unnamed on purpose. If the underlying types add more members,
	// we want them to be explicitly added to the serializableCluster.
	return &Cluster{
		&cluster.Cluster{
			s.ContentIDs,
			s.Segments,
			&cluster.GPDBExecutor{},
		},
		s.BinDir,
		s.Version,
	}
}

func (c *Cluster) MarshalJSON() ([]byte, error) {
	// See notes for serializableCluster for why we override the standard
	// marshal operation.
	return json.Marshal(newSerializableCluster(c))
}

func (c *Cluster) UnmarshalJSON(b []byte) error {
	// See notes for serializableCluster for why we override the standard
	// unmarshal operation.
	s := new(serializableCluster)

	err := json.Unmarshal(b, s)
	if err != nil {
		return err
	}

	*c = *s.cluster()
	return nil
}

// ErrUnknownHost can be returned by Cluster.SegmentsOn.
var ErrUnknownHost = xerrors.New("no such host in cluster")

type UnknownHostError struct {
	hostname string
}

func (u UnknownHostError) Error() string {
	return fmt.Sprintf("cluster has no segments on host %q", u.hostname)
}

func (u UnknownHostError) Is(err error) bool {
	return err == ErrUnknownHost
}

// SegmentsOn returns the configurations of segments that are running on a given
// host excluding the master. An error of type ErrUnknownHost will be returned
// for unknown hostnames.
func (c Cluster) SegmentsOn(hostname string) ([]cluster.SegConfig, error) {
	var segments []cluster.SegConfig
	for _, segment := range c.Segments {
		if segment.Hostname == hostname && segment.ContentID != -1 {
			segments = append(segments, segment)
		}
	}

	if len(segments) == 0 {
		return nil, UnknownHostError{hostname}
	}

	return segments, nil
}
