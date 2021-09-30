// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

const MasterDbid = 1

type Cluster struct {
	Destination idl.ClusterDestination

	// Primaries contains the primary SegConfigs, keyed by content ID.
	Primaries map[int]SegConfig

	// Mirrors contains any mirror SegConfigs, keyed by content ID.
	Mirrors map[int]SegConfig

	Tablespaces Tablespaces

	GPHome         string
	Version        semver.Version
	CatalogVersion string
}

// ClusterFromDB will create a Cluster by querying the passed DBConn for
// information. You must pass the cluster's gphome, since it cannot be
// divined from the database.
func ClusterFromDB(db *sql.DB, version semver.Version, gphome string, destination idl.ClusterDestination) (Cluster, error) {
	segments, err := GetSegmentConfiguration(db, version)
	if err != nil {
		return Cluster{}, xerrors.Errorf("querying gp_segment_configuration: %w", err)
	}

	cluster, err := NewCluster(segments)
	if err != nil {
		return Cluster{}, err
	}

	cluster.Destination = destination
	cluster.Version = version
	cluster.GPHome = gphome

	return cluster, nil
}

func NewCluster(segments SegConfigs) (Cluster, error) {
	cluster := Cluster{}
	cluster.Primaries = make(map[int]SegConfig)
	cluster.Mirrors = make(map[int]SegConfig)

	for _, seg := range segments {
		if seg.IsPrimary() || seg.IsMaster() {
			cluster.Primaries[seg.ContentID] = seg
			continue
		}

		if seg.IsMirror() || seg.IsStandby() {
			cluster.Mirrors[seg.ContentID] = seg
			continue
		}

		return Cluster{}, errors.New("Expected role to be primary or mirror, but found none when creating cluster.")
	}

	return cluster, nil
}

func (c *Cluster) ExcludingMasterOrStandby() SegConfigs {
	segs := SegConfigs{}

	for _, seg := range c.Primaries {
		if seg.IsMaster() {
			continue
		}

		segs = append(segs, seg)
	}

	for _, seg := range c.Mirrors {
		if seg.IsStandby() {
			continue
		}

		segs = append(segs, seg)
	}

	return segs
}

func (c *Cluster) Master() SegConfig {
	return c.Primaries[-1]
}

func (c *Cluster) MasterDataDir() string {
	return c.Master().DataDir
}

func (c *Cluster) MasterPort() int {
	return c.Master().Port
}

func (c *Cluster) MasterHostname() string {
	return c.Master().Hostname
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
	return c.Standby().Port
}

func (c *Cluster) StandbyHostname() string {
	return c.Standby().Hostname
}

func (c *Cluster) StandbyDataDir() string {
	return c.Standby().DataDir
}

// Returns true if we have at least one mirror that is not a standby
func (c *Cluster) HasMirrors() bool {
	if len(c.Mirrors) == 0 {
		return false
	}

	for _, mirror := range c.Mirrors {
		if mirror.IsStandby() {
			continue
		}

		return true
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

// SelectSegments returns a list of all segments that match the given selector
// function.
func (c Cluster) SelectSegments(selector func(*SegConfig) bool) SegConfigs {
	var matches SegConfigs

	for _, seg := range c.Primaries {
		if selector(&seg) {
			matches = append(matches, seg)
		}
	}

	for _, seg := range c.Mirrors {
		if selector(&seg) {
			matches = append(matches, seg)
		}
	}

	return matches
}

func (c *Cluster) Start(stream step.OutStreams) error {
	err := c.RunGreenplumCmd(stream, "gpstart", "-a", "-d", c.MasterDataDir())
	if err != nil {
		return xerrors.Errorf("starting %s cluster: %w", strings.ToLower(c.Destination.String()), err)
	}

	return nil
}

func (c *Cluster) StartMasterOnly(stream step.OutStreams) error {
	err := c.RunGreenplumCmd(stream, "gpstart", "-a", "-m", "-d", c.MasterDataDir())
	if err != nil {
		return xerrors.Errorf("starting %s cluster in master only mode: %w", strings.ToLower(c.Destination.String()), err)
	}

	return nil
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
		return errors.New(fmt.Sprintf("Failed to stop %s cluster. Master is already stopped.", strings.ToLower(c.Destination.String())))
	}

	err = c.RunGreenplumCmd(stream, "gpstop", "-a", "-d", c.MasterDataDir())
	if err != nil {
		return xerrors.Errorf("stopping %s cluster: %w", strings.ToLower(c.Destination.String()), err)
	}

	return nil
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
		return errors.New(fmt.Sprintf("Failed to stop %s cluster in master only mode. Master is already stopped.", strings.ToLower(c.Destination.String())))
	}

	err = c.RunGreenplumCmd(stream, "gpstop", "-a", "-m", "-d", c.MasterDataDir())
	if err != nil {
		return xerrors.Errorf("stopping %s cluster: %w", strings.ToLower(c.Destination.String()), err)
	}

	return nil
}

var isMasterRunningCommand = exec.Command

// XXX: for internal testing only
func SetIsMasterRunningCommand(command exectest.Command) {
	isMasterRunningCommand = command
}

// XXX: for internal testing only
func ResetIsMasterRunningCommand() {
	isMasterRunningCommand = exec.Command
}

func (c *Cluster) IsMasterRunning(stream step.OutStreams) (bool, error) {
	path := filepath.Join(c.MasterDataDir(), "postmaster.pid")
	exist, err := upgrade.PathExist(path)
	if err != nil {
		return false, err
	}

	if !exist {
		return false, err
	}

	cmd := isMasterRunningCommand("pgrep", "-F", path)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	gplog.Debug("checking if master process is running with %s", cmd.String())

	err = cmd.Run()
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

var greenplumCommand = exec.Command

// XXX: for internal testing only
func SetGreenplumCommand(command exectest.Command) {
	greenplumCommand = command
}

// XXX: for internal testing only
func ResetGreenplumCommand() {
	greenplumCommand = exec.Command
}

func (c *Cluster) RunGreenplumCmd(streams step.OutStreams, utility string, args ...string) error {
	return c.runGreenplumCommand(streams, utility, args, nil)
}

func (c *Cluster) RunGreenplumCmdWithEnvironment(streams step.OutStreams, utility string, args []string, envs []string) error {
	return c.runGreenplumCommand(streams, utility, args, envs)
}

func (c *Cluster) runGreenplumCommand(streams step.OutStreams, utility string, args []string, envs []string) error {
	path := filepath.Join(c.GPHome, "bin", utility)
	args = append([]string{path}, args...)

	cmd := greenplumCommand("bash", "-c", fmt.Sprintf("source %s/greenplum_path.sh && %s", c.GPHome, shellquote.Join(args...)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", "MASTER_DATA_DIRECTORY", c.MasterDataDir()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", "PGPORT", c.MasterPort()))
	cmd.Env = append(cmd.Env, envs...)

	cmd.Stdout = streams.Stdout()
	cmd.Stderr = streams.Stderr()

	gplog.Info("executing: %s", cmd.String())
	return cmd.Run()
}

// WaitForClusterToBeReady waits until the timeout for all segments to be up,
// in their preferred role, and synchronized.
func (c *Cluster) WaitForClusterToBeReady(conn *Conn) error {
	destination := ToTarget()
	if c.Destination == idl.ClusterDestination_SOURCE {
		destination = ToSource()
	}

	options := []Option{
		destination,
		Port(c.MasterPort()),
	}

	db, err := sql.Open("pgx", conn.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	return WaitForSegments(db, 5*time.Minute, c)
}

func WaitForSegments(db *sql.DB, timeout time.Duration, cluster *Cluster) error {
	startTime := time.Now()
	for {
		if cluster.Version.Major > 5 {
			rows, err := db.Query("SELECT gp_request_fts_probe_scan();")
			if err != nil {
				return xerrors.Errorf("requesting gp_request_fts_probe_scan: %w", err)
			}

			if err := rows.Close(); err != nil {
				return xerrors.Errorf("closing rows for gp_request_fts_probe_scan: %w", err)
			}
		}

		ready, err := areSegmentsReady(db, cluster)
		if err != nil {
			return err
		}

		if ready {
			return nil
		}

		if time.Since(startTime) > timeout {
			return xerrors.Errorf("%s timeout exceeded waiting for all segments to be up, in their preferred roles, and synchronized.", timeout)
		}

		time.Sleep(time.Second)
	}
}

func areSegmentsReady(db *sql.DB, cluster *Cluster) (bool, error) {
	var segments int

	// check gp_segment_configuration on segments
	whereClause := "AND mode = 's'"
	if !cluster.HasMirrors() {
		whereClause = ""
	}

	row := db.QueryRow(`SELECT COUNT(*) FROM gp_segment_configuration 
WHERE content > -1 AND status = 'u' AND (role = preferred_role) ` + whereClause)

	if err := row.Scan(&segments); err != nil {
		if err == sql.ErrNoRows {
			gplog.Debug("no rows found when querying gp_segment_configuration")
			return false, nil
		}

		return false, xerrors.Errorf("querying gp_segment_configuration: %w", err)
	}

	if segments != len(cluster.ExcludingMasterOrStandby()) {
		return false, nil
	}

	// check gp_stat_replication for the standby. Note, gp_stat_replication does not exist in GPDB 5.
	if cluster.Version.Major == 5 || !cluster.HasStandby() {
		return true, nil
	}

	row = db.QueryRow("SELECT COUNT(*) FROM gp_stat_replication WHERE gp_segment_id = -1 AND state = 'streaming' AND sent_location = flush_location;")
	if err := row.Scan(&segments); err != nil {
		if err == sql.ErrNoRows {
			gplog.Debug("no rows found when querying gp_stat_replication")
			return false, nil
		}

		return false, xerrors.Errorf("querying gp_stat_replication: %w", err)
	}

	if segments != 1 {
		return false, nil
	}

	return true, nil
}
