// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
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

const CoordinatorDbid = 1

type Cluster struct {
	Destination idl.ClusterDestination

	// Primaries contains the primary SegConfigs, keyed by content ID.
	Primaries ContentToSegConfig

	// Mirrors contains any mirror SegConfigs, keyed by content ID.
	Mirrors ContentToSegConfig

	Tablespaces Tablespaces

	GPHome         string
	Version        semver.Version
	CatalogVersion string
}

type ContentToSegConfig map[int]SegConfig

func (c ContentToSegConfig) ExcludingCoordinator() ContentToSegConfig {
	return c.excludingCoordinatorOrStandby()
}

func (c ContentToSegConfig) ExcludingStandby() ContentToSegConfig {
	return c.excludingCoordinatorOrStandby()
}

func (c ContentToSegConfig) excludingCoordinatorOrStandby() ContentToSegConfig {
	segsExcludingCoordinatorOrStandby := make(ContentToSegConfig)

	for _, seg := range c {
		if seg.IsStandby() || seg.IsCoordinator() {
			continue
		}

		segsExcludingCoordinatorOrStandby[seg.ContentID] = seg
	}

	return segsExcludingCoordinatorOrStandby
}

// ClusterFromDB will create a Cluster by querying the passed DBConn for
// information. You must pass the cluster's gphome, since it cannot be
// divined from the database.
func ClusterFromDB(db *sql.DB, gphome string, destination idl.ClusterDestination) (Cluster, error) {
	version, err := Version(gphome)
	if err != nil {
		return Cluster{}, err
	}

	segments, err := GetSegmentConfiguration(db, semver.MustParse(version))
	if err != nil {
		return Cluster{}, xerrors.Errorf("querying gp_segment_configuration: %w", err)
	}

	cluster, err := NewCluster(segments)
	if err != nil {
		return Cluster{}, err
	}

	cluster.Destination = destination
	cluster.Version = semver.MustParse(version)
	cluster.GPHome = gphome

	return cluster, nil
}

func NewCluster(segments SegConfigs) (Cluster, error) {
	cluster := Cluster{}
	cluster.Primaries = make(map[int]SegConfig)
	cluster.Mirrors = make(map[int]SegConfig)

	for _, seg := range segments {
		if seg.IsPrimary() || seg.IsCoordinator() {
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

func (c *Cluster) ExcludingCoordinatorOrStandby() SegConfigs {
	segs := SegConfigs{}

	for _, seg := range c.Primaries {
		if seg.IsCoordinator() {
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

func (c *Cluster) Coordinator() SegConfig {
	return c.Primaries[-1]
}

func (c *Cluster) CoordinatorDataDir() string {
	return c.Coordinator().DataDir
}

func (c *Cluster) CoordinatorPort() int {
	return c.Coordinator().Port
}

func (c *Cluster) CoordinatorHostname() string {
	return c.Coordinator().Hostname
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
		// Ignore the coordinator.
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
	err := c.RunGreenplumCmd(stream, "gpstart", "-a", "-d", c.CoordinatorDataDir())
	if err != nil {
		return xerrors.Errorf("starting %s cluster: %w", c.Destination, err)
	}

	return nil
}

func (c *Cluster) StartCoordinatorOnly(stream step.OutStreams) error {
	err := c.RunGreenplumCmd(stream, "gpstart", "-a", "-m", "-d", c.CoordinatorDataDir())
	if err != nil {
		return xerrors.Errorf("starting %s cluster in master only mode: %w", c.Destination, err)
	}

	return nil
}

func (c *Cluster) Stop(stream step.OutStreams) error {
	// TODO: why can't we call IsCoordinatorRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	running, err := c.IsCoordinatorRunning(stream)
	if err != nil {
		return err
	}

	if !running {
		return errors.New(fmt.Sprintf("Failed to stop %s cluster. Master is already stopped.", c.Destination))
	}

	err = c.RunGreenplumCmd(stream, "gpstop", "-a", "-d", c.CoordinatorDataDir())
	if err != nil {
		return xerrors.Errorf("stopping %s cluster: %w", c.Destination, err)
	}

	return nil
}

func (c *Cluster) StopCoordinatorOnly(stream step.OutStreams) error {
	// TODO: why can't we call IsCoordinatorRunning for the !stop case?  If we do, we get this on the pipeline:
	// Usage: pgrep [-flvx] [-d DELIM] [-n|-o] [-P PPIDLIST] [-g PGRPLIST] [-s SIDLIST]
	// [-u EUIDLIST] [-U UIDLIST] [-G GIDLIST] [-t TERMLIST] [PATTERN]
	//  pgrep: pidfile not valid
	running, err := c.IsCoordinatorRunning(stream)
	if err != nil {
		return err
	}

	if !running {
		return errors.New(fmt.Sprintf("Failed to stop %s cluster in master only mode. Master is already stopped.", c.Destination))
	}

	err = c.RunGreenplumCmd(stream, "gpstop", "-a", "-m", "-d", c.CoordinatorDataDir())
	if err != nil {
		return xerrors.Errorf("stopping %s cluster: %w", c.Destination, err)
	}

	return nil
}

var isCoordinatorRunningCommand = exec.Command

// XXX: for internal testing only
func SetIsCoordinatorRunningCommand(command exectest.Command) {
	isCoordinatorRunningCommand = command
}

// XXX: for internal testing only
func ResetIsCoordinatorRunningCommand() {
	isCoordinatorRunningCommand = exec.Command
}

func (c *Cluster) IsCoordinatorRunning(stream step.OutStreams) (bool, error) {
	path := filepath.Join(c.CoordinatorDataDir(), "postmaster.pid")
	exist, err := upgrade.PathExist(path)
	if err != nil {
		return false, err
	}

	if !exist {
		return false, err
	}

	cmd := isCoordinatorRunningCommand("pgrep", "-F", path)

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
	cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", "MASTER_DATA_DIRECTORY", c.CoordinatorDataDir()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", "PGPORT", c.CoordinatorPort()))
	cmd.Env = append(cmd.Env, envs...)

	cmd.Stdout = streams.Stdout()
	cmd.Stderr = streams.Stderr()

	gplog.Info("executing: %s", cmd.String())
	return cmd.Run()
}

func (c *Cluster) CheckActiveConnections() error {
	db, err := sql.Open("pgx", c.Connection())
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	return QueryPgStatActivity(db, c)
}

// WaitForClusterToBeReady waits until the timeout for all segments to be up,
// in their preferred role, and synchronized.
func (c *Cluster) WaitForClusterToBeReady() error {
	db, err := sql.Open("pgx", c.Connection())
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
