// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var ErrUnknownCatalogVersion = errors.New("pg_controldata output is missing catalog version")

func (s *Server) GenerateInitsystemConfig() error {
	options := []greenplum.Option{
		greenplum.ToSource(),
		greenplum.Port(s.Source.MasterPort()),
	}

	db, err := sql.Open("pgx", s.Connection.URI(options...))
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errorlist.Append(err, cErr)
		}
	}()

	return s.writeConf(db)
}

func (s *Server) initsystemConfPath() string {
	return filepath.Join(s.StateDir, "gpinitsystem_config")
}

func (s *Server) writeConf(db *sql.DB) error {
	gpinitsystemConfig, err := CreateInitialInitsystemConfig(s.IntermediateTarget.MasterDataDir(), s.UseHbaHostnames)
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, s.Source.Version, db)
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = WriteSegmentArray(gpinitsystemConfig, s.IntermediateTarget)
	if err != nil {
		return xerrors.Errorf("generating segment array: %w", err)
	}

	return WriteInitsystemFile(gpinitsystemConfig, s.initsystemConfPath())
}

func (s *Server) RemoveIntermediateTargetCluster(streams step.OutStreams) error {
	if reflect.DeepEqual(s.IntermediateTarget, greenplum.Cluster{}) {
		return nil
	}

	running, err := s.IntermediateTarget.IsMasterRunning(streams)
	if err != nil {
		return err
	}

	if running {
		if err := s.IntermediateTarget.Stop(streams); err != nil {
			return err
		}
	}

	err = DeleteMasterAndPrimaryDataDirectories(streams, s.agentConns, s.IntermediateTarget)
	if err != nil {
		return xerrors.Errorf("deleting target cluster data directories: %w", err)
	}

	return nil
}

func (s *Server) InitTargetCluster(stream step.OutStreams) error {
	return RunInitsystemForTargetCluster(stream,
		s.IntermediateTarget.GPHome, s.initsystemConfPath(), s.IntermediateTarget.Version)
}

func GetCheckpointSegmentsAndEncoding(gpinitsystemConfig []string, version semver.Version, db *sql.DB) ([]string, error) {
	var encoding string
	err := db.QueryRow("SELECT current_setting('server_encoding') AS string").Scan(&encoding)
	if err != nil {
		return gpinitsystemConfig, xerrors.Errorf("retrieve server encoding: %w", err)
	}

	gpinitsystemConfig = append(gpinitsystemConfig, fmt.Sprintf("ENCODING=%s", encoding))

	// The 7X guc max_wal_size supersedes checkpoint_segments and its default value is sufficient.
	if version.Major < 7 {
		var checkpointSegments string
		err := db.QueryRow("SELECT current_setting('checkpoint_segments') AS string").Scan(&checkpointSegments)
		if err != nil {
			return gpinitsystemConfig, xerrors.Errorf("retrieve checkpoint segments: %w", err)
		}

		gpinitsystemConfig = append(gpinitsystemConfig, fmt.Sprintf("CHECK_POINT_SEGMENTS=%s", checkpointSegments))
	}

	return gpinitsystemConfig, nil
}

func CreateInitialInitsystemConfig(targetMasterDataDir string, useHbaHostnames bool) ([]string, error) {
	gpinitsystemConfig := []string{`ARRAY_NAME="gp_upgrade cluster"`}

	segPrefix, err := GetMasterSegPrefix(targetMasterDataDir)
	if err != nil {
		return gpinitsystemConfig, xerrors.Errorf("determine master segment prefix: %w", err)
	}

	hbaHostnames := "0"
	if useHbaHostnames {
		hbaHostnames = "1"
	}

	gpinitsystemConfig = append(gpinitsystemConfig, "SEG_PREFIX="+segPrefix, "TRUSTED_SHELL=ssh", "HBA_HOSTNAMES="+hbaHostnames)

	return gpinitsystemConfig, nil
}

func WriteInitsystemFile(gpinitsystemConfig []string, gpinitsystemFilepath string) error {
	gpinitsystemContents := []byte(strings.Join(gpinitsystemConfig, "\n"))

	err := ioutil.WriteFile(gpinitsystemFilepath, gpinitsystemContents, 0644)
	if err != nil {
		return xerrors.Errorf("write gpinitsystem_config file: %w", err)
	}
	return nil
}

func WriteSegmentArray(config []string, intermediateTarget *greenplum.Cluster) ([]string, error) {
	master := intermediateTarget.Master()
	config = append(config,
		fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%s~%d~%s~%d~%d",
			master.Hostname,
			master.Hostname,
			master.Port,
			master.DataDir,
			master.DbID,
			master.ContentID,
		),
	)

	config = append(config, "declare -a PRIMARY_ARRAY=(")
	for _, segment := range intermediateTarget.Primaries {
		if segment.ContentID == -1 {
			continue
		}

		config = append(config,
			fmt.Sprintf("\t%s~%s~%d~%s~%d~%d",
				segment.Hostname,
				segment.Hostname,
				segment.Port,
				segment.DataDir,
				segment.DbID,
				segment.ContentID,
			),
		)
	}
	config = append(config, ")")

	return config, nil
}

func RunInitsystemForTargetCluster(stream step.OutStreams, gpHome, configPath string, version semver.Version) error {
	// TODO: migrate this implementation to greenplum.Runner.

	args := "-a -I " + configPath
	if version.Major < 7 {
		// For 6X we add --ignore-warnings to gpinitsystem to return 0 on
		// warnings and 1 on errors. 7X and later does this by default.
		args += " --ignore-warnings"
	}

	script := fmt.Sprintf("source %[1]s/greenplum_path.sh && %[1]s/bin/gpinitsystem %[2]s",
		gpHome,
		args,
	)
	cmd := execCommand("bash", "-c", script)

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

	// Sanitize the child environment. The sourcing of greenplum_path.sh will
	// give us back almost everything we need, but it's important not to put a
	// previous installation's ambient environment into the mix.
	//
	// gpinitsystem unfortunately relies on a few envvars for logging purposes;
	// otherwise, we could clear the environment completely.
	cmd.Env = filterEnv([]string{
		"HOME",
		"USER",
		"LOGNAME",
	})

	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("gpinitsystem: %w", err)
	}

	return nil
}

// filterEnv selects only the specified variables from the environment and
// returns those key/value pairs, in the key=value format expected by
// os/exec.Cmd.Env.
func filterEnv(keys []string) []string {
	var env []string

	for _, key := range keys {
		val, ok := os.LookupEnv(key)
		if !ok {
			continue
		}

		env = append(env, fmt.Sprintf("%s=%s", key, val))
	}

	return env
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

func GetCatalogVersion(stream step.OutStreams, gphome, datadir string) (string, error) {
	utility := filepath.Join(gphome, "bin", "pg_controldata")
	cmd := execCommand(utility, datadir)

	// Buffer stdout to parse pg_controldata
	stdout := new(bytes.Buffer)
	tee := io.MultiWriter(stream.Stdout(), stdout)

	cmd.Stdout = tee
	cmd.Stderr = stream.Stderr()

	gplog.Debug("determining catalog version with %s", cmd.String())
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// parse pg_control data
	var version string
	prefix := "Catalog version number:"

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimPrefix(line, prefix)
			version = strings.TrimSpace(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", xerrors.Errorf("scanning pg_controldata: %w", err)
	}

	if version == "" {
		return "", ErrUnknownCatalogVersion
	}

	return version, nil
}
