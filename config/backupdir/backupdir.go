// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package backupdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

type BackupDirs struct {
	// Can collapse the CoordinatorBackupDir into AgentHostsToBackupDir once we run an agent on the coordinator
	// to simplify various logic.
	CoordinatorBackupDir  string
	AgentHostsToBackupDir AgentHostsToBackupDir
}

type AgentHostsToBackupDir map[string]string

// ParseParentBackupDirs parses either a single directory or multiple
// host:directory pairs. To specify a single directory across all hosts set a
// single directory such as /dir. To specify different directories for each host
// use the form "host1:/dir1,host2:/dir2,host3:/dir3" where the first host must
// be the coordinator. Defaults to the parent directory of each primary data
// directory on each primary host.
// NOTE: We parse in the hub rather than the CLI since we need to know all hosts
// to fill in the agent hosts backup directories if the user specifies a single
// directory and no hosts.
func ParseParentBackupDirs(input string, cluster greenplum.Cluster) (BackupDirs, error) {
	input = strings.TrimSpace(input)
	backupDirs := BackupDirs{}
	backupDirs.AgentHostsToBackupDir = make(AgentHostsToBackupDir)

	// set default backup directories
	if input == "" {
		backupDirs.CoordinatorBackupDir = filepath.Join(filepath.Dir(cluster.CoordinatorDataDir()), string(os.PathSeparator), ".gpupgrade")

		for _, seg := range cluster.Primaries.ExcludingCoordinator() {
			backupDirs.AgentHostsToBackupDir[seg.Hostname] = filepath.Join(filepath.Dir(seg.DataDir), string(os.PathSeparator), ".gpupgrade")
		}

		return backupDirs, nil
	}

	// parse single backup directory across all hosts
	if !strings.ContainsAny(input, ",:") {
		backupDir := filepath.Join(filepath.Clean(input), ".gpupgrade")

		backupDirs.CoordinatorBackupDir = backupDir
		for _, seg := range cluster.ExcludingCoordinatorOrStandby() {
			backupDirs.AgentHostsToBackupDir[seg.Hostname] = backupDir
		}

		return backupDirs, nil
	}

	// parse multiple backup directories across all hosts
	parseCoordinator := true
	parts := strings.Split(input, ",")
	for _, pair := range parts {
		hostBackupParts := strings.Split(strings.TrimSpace(pair), ":")
		host := strings.TrimSpace(hostBackupParts[0])
		backupDir := filepath.Join(filepath.Clean(strings.TrimSpace(hostBackupParts[1])), ".gpupgrade")

		if parseCoordinator {
			backupDirs.CoordinatorBackupDir = backupDir
			parseCoordinator = false
			continue
		}

		backupDirs.AgentHostsToBackupDir[host] = backupDir
	}

	// ensure all hosts have been specified when multiple backup directories specified
	var missingHosts []string
	for _, host := range cluster.PrimaryHostnames() {
		if _, ok := backupDirs.AgentHostsToBackupDir[host]; !ok {
			missingHosts = append(missingHosts, host)
		}
	}

	if len(missingHosts) > 0 {
		return BackupDirs{}, newMissingHostInParentBackupDirsError(input, missingHosts)
	}

	return backupDirs, nil
}

var ErrMissingHostInParentBackupDirs = errors.New("missing host in parent backup directories")

type MissingHostInParentBackupDirsError struct {
	Input        string
	MissingHosts []string
}

func newMissingHostInParentBackupDirsError(input string, missingHosts []string) *MissingHostInParentBackupDirsError {
	return &MissingHostInParentBackupDirsError{Input: input, MissingHosts: missingHosts}
}

func (m *MissingHostInParentBackupDirsError) Error() string {
	return fmt.Sprintf("expected host %q to be specified in %q when parsing parent_backup_dirs", m.MissingHosts, m.Input)
}

func (m *MissingHostInParentBackupDirsError) Is(err error) bool {
	return err == ErrMissingHostInParentBackupDirs
}
