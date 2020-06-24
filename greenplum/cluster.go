// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

var isPostmasterRunningCmd = exec.Command
var startStopCmd = exec.Command

const MasterDbid = 1

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

// ClusterFromDB will create a Cluster by querying the passed DBConn for
// information. You must pass the cluster's binary directory, since it cannot be
// divined from the database.
func ClusterFromDB(conn *dbconn.DBConn, binDir string) (*Cluster, error) {
	err := conn.Connect(1)
	if err != nil {
		return nil, xerrors.Errorf("connect to cluster: %w", err)
	}
	defer conn.Close()

	segments, err := GetSegmentConfiguration(conn)
	if err != nil {
		return nil, xerrors.Errorf("retrieve segment configuration: %w", err)
	}

	c, err := NewCluster(segments)
	if err != nil {
		return nil, err
	}

	c.Version = conn.Version
	c.BinDir = binDir

	return c, nil
}

func (c *Cluster) Master() SegConfig {
	return c.Primaries[-1]
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

// the standby might not exist, so it is the caller's responsibility
func (c *Cluster) Standby() SegConfig {
	return c.Mirrors[-1]
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

// Returns true if we have at least one mirror that is not a standby
func (c *Cluster) HasMirrors() bool {
	for contentID := range c.ContentIDs {
		if _, ok := c.Mirrors[contentID]; ok && contentID != -1 {
			return true
		}
	}
	return false
}

func (c *Cluster) HasAllMirrorsAndStandby() bool {
	for content := range c.Primaries {
		if _, ok := c.Mirrors[content]; !ok {
			return false
		}
	}

	return true
}

// XXX This does not provide mirror hostnames yet.
func (c *Cluster) GetHostnames() []string {
	hostnameMap := make(map[string]bool)
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
	hostnames := make(map[string]bool)
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

func FilterSegmentsOnHost(segmentsToFilter []SegConfig, hostname string) []SegConfig {
	var segments []SegConfig
	for _, segment := range segmentsToFilter {
		if segment.Hostname == hostname {
			segments = append(segments, segment)
		}
	}

	return segments
}

// SelectSegments returns a list of all segments that match the given selector
// function. Segments are visited in order of ascending content ID (primaries
// before mirrors).
func (c Cluster) SelectSegments(selector func(*SegConfig) bool) []SegConfig {
	var matches []SegConfig

	for _, content := range c.ContentIDs {
		seg := c.Primaries[content]
		if selector(&seg) {
			matches = append(matches, seg)
		}

		if seg, ok := c.Mirrors[content]; ok && selector(&seg) {
			matches = append(matches, seg)
		}
	}

	return matches
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

func (c *Cluster) GetContentList() []int {
	return c.ContentIDs
}

func (c *Cluster) GetDbidForContent(contentID int) int {
	return c.Primaries[contentID].DbID
}

func (c *Cluster) GetPortForContent(contentID int) int {
	return c.Primaries[contentID].Port
}

func (c *Cluster) GetHostForContent(contentID int) string {
	return c.Primaries[contentID].Hostname
}

func (c *Cluster) GetDirForContent(contentID int) string {
	return c.Primaries[contentID].DataDir
}

func (c *Cluster) Start(stream step.OutStreams) error {
	return runStartStopCmd(stream, c.BinDir, fmt.Sprintf("gpstart -a -d %[1]s", c.MasterDataDir()))
}

func (c *Cluster) Stop(stream step.OutStreams) error {
	// TODO: why can't we call IsMasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	running, err := c.IsMasterRunning(stream)
	if err != nil {
		return err
	}

	if !running {
		return errors.New("master is already stopped")
	}

	return runStartStopCmd(stream, c.BinDir, fmt.Sprintf("gpstop -a -d %[1]s", c.MasterDataDir()))
}

func (c *Cluster) StartMasterOnly(stream step.OutStreams) error {
	return runStartStopCmd(stream, c.BinDir, fmt.Sprintf("gpstart -m -a -d %[1]s", c.MasterDataDir()))
}

func (c *Cluster) StopMasterOnly(stream step.OutStreams) error {
	// TODO: why can't we call IsMasterRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	running, err := c.IsMasterRunning(stream)
	if err != nil {
		return err
	}

	if !running {
		return errors.New("master is already stopped")
	}

	return runStartStopCmd(stream, c.BinDir, fmt.Sprintf("gpstop -m -a -d %[1]s", c.MasterDataDir()))
}

func runStartStopCmd(stream step.OutStreams, binDir, command string) error {
	commandWithEnv := fmt.Sprintf("source %[1]s/../greenplum_path.sh && %[1]s/%[2]s",
		binDir,
		command)

	cmd := startStopCmd("bash", "-c", commandWithEnv)
	gplog.Info("running command: %q", cmd)
	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()
	return cmd.Run()
}

// IsMasterRunning returns whether the cluster's master process is running.
func (c *Cluster) IsMasterRunning(stream step.OutStreams) (bool, error) {
	path := filepath.Join(c.MasterDataDir(), "postmaster.pid")
	if !upgrade.PathExists(path) {
		return false, nil
	}

	cmd := isPostmasterRunningCmd("pgrep", "-F", path)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	gplog.Debug("checking if master process is running with %s", cmd.String())

	err := cmd.Run()
	var exitErr *exec.ExitError
	if xerrors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			// No processes were matched
			return false, nil
		}
	}

	if err != nil {
		return false, xerrors.Errorf("checking for postmaster process: %w", err)
	}

	return true, nil
}
