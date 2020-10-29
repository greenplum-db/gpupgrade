// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
)

var ErrUnknownCatalogVersion = errors.New("pg_controldata output is missing catalog version")

func (s *Server) GenerateInitsystemConfig() error {
	sourceDBConn := db.NewDBConn("localhost", int(s.Source.MasterPort()), "template1")
	return s.writeConf(sourceDBConn)
}

func (s *Server) initsystemConfPath() string {
	return filepath.Join(s.StateDir, "gpinitsystem_config")
}

func (s *Server) writeConf(sourceDBConn *dbconn.DBConn) error {
	err := sourceDBConn.Connect(1)
	if err != nil {
		return xerrors.Errorf("connect to database: %w", err)
	}
	defer sourceDBConn.Close()

	gpinitsystemConfig, err := CreateInitialInitsystemConfig(s.TargetInitializeConfig.Master.DataDir, s.UseHbaHostnames)
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, sourceDBConn)
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = WriteSegmentArray(gpinitsystemConfig, s.TargetInitializeConfig)
	if err != nil {
		return xerrors.Errorf("generating segment array: %w", err)
	}

	return WriteInitsystemFile(gpinitsystemConfig, s.initsystemConfPath())
}

// CreateTargetCluster runs gpinitsystem using the server's
// TargetInitializeConfig, then fills in the Target cluster and persists it to
// disk.
func (s *Server) CreateTargetCluster(stream step.OutStreams) error {
	err := s.InitTargetCluster(stream)
	if err != nil {
		return err
	}

	conn := db.NewDBConn("localhost", s.TargetInitializeConfig.Master.Port, "template1")
	defer conn.Close()

	s.Target, err = greenplum.ClusterFromDB(conn, s.TargetGPHome)
	if err != nil {
		return xerrors.Errorf("retrieve target configuration: %w", err)
	}

	if err := s.SaveConfig(); err != nil {
		return err
	}

	return nil
}

func (s *Server) InitTargetCluster(stream step.OutStreams) error {
	version, err := greenplum.GPHomeVersion(s.TargetGPHome)
	if err != nil {
		return err
	}

	return RunInitsystemForTargetCluster(stream,
		s.TargetGPHome, s.initsystemConfPath(), version)
}

func GetCheckpointSegmentsAndEncoding(gpinitsystemConfig []string, dbConnector *dbconn.DBConn) ([]string, error) {
	checkpointSegments, err := dbconn.SelectString(dbConnector, "SELECT current_setting('checkpoint_segments') AS string")
	if err != nil {
		return gpinitsystemConfig, xerrors.Errorf("retrieve checkpoint segments: %w", err)
	}
	encoding, err := dbconn.SelectString(dbConnector, "SELECT current_setting('server_encoding') AS string")
	if err != nil {
		return gpinitsystemConfig, xerrors.Errorf("retrieve server encoding: %w", err)
	}
	gpinitsystemConfig = append(gpinitsystemConfig,
		fmt.Sprintf("CHECK_POINT_SEGMENTS=%s", checkpointSegments),
		fmt.Sprintf("ENCODING=%s", encoding))
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

func WriteSegmentArray(config []string, targetInitializeConfig InitializeConfig) ([]string, error) {
	//Partition segments by host in order to correctly assign ports.
	if targetInitializeConfig.Master == (greenplum.SegConfig{}) {
		return nil, errors.New("source cluster contains no master segment")
	}

	master := targetInitializeConfig.Master
	config = append(config,
		fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%d~%s~%d~%d",
			master.Hostname,
			master.Port,
			master.DataDir,
			master.DbID,
			master.ContentID,
		),
	)

	config = append(config, "declare -a PRIMARY_ARRAY=(")
	for _, segment := range targetInitializeConfig.Primaries {
		config = append(config,
			fmt.Sprintf("\t%s~%d~%s~%d~%d",
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
