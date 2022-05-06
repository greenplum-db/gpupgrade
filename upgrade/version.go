// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func LocalVersion() (string, error) {
	return version("")
}

func RemoteVersion(host string) (string, error) {
	return version(host)
}

var versionCommand = exec.Command
var remoteVersionCommand = exec.Command

// XXX: for internal testing only
func SetLocalVersionCommand(command exectest.Command) {
	versionCommand = command
}

func SetRemoteVersionCommand(command exectest.Command) {
	remoteVersionCommand = command
}

// XXX: for internal testing only
func ResetLocalVersionCommand() {
	versionCommand = exec.Command
}

func ResetRemoteVersionCommand() {
	remoteVersionCommand = exec.Command
}

func version(host string) (string, error) {
	gpupgradePath, err := utils.GetGpupgradePath()
	if err != nil {
		return "", xerrors.Errorf("getting gpupgrade binary path: %w", err)
	}

	name := gpupgradePath
	args := []string{"version", "--format", "oneline"}
	if host != "" {
		versionCommand = remoteVersionCommand
		name = "ssh"
		args = []string{"-q", host, fmt.Sprintf(`bash -c "%s version --format oneline"`, gpupgradePath)}
	}

	cmd := versionCommand(name, args...)
	gplog.Debug(cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", xerrors.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	gplog.Debug("output: %q", output)

	return string(output), nil
}

func EnsureGpupgradeVersionsMatch(agentHosts []string) error {
	type HostVersion struct {
		host    string
		version string
		err     error
	}

	hubVersion, err := LocalVersion()
	if err != nil {
		return xerrors.Errorf("hub version: %w", err)
	}

	hostVersions := make(chan HostVersion, len(agentHosts))
	var wg sync.WaitGroup

	for _, host := range agentHosts {
		host := host

		wg.Add(1)
		go func() {
			defer wg.Done()

			version, err := RemoteVersion(host)
			hostVersions <- HostVersion{host: host, version: version, err: err}
		}()
	}

	wg.Wait()
	close(hostVersions)

	mismatched := make(MismatchedVersions)
	for agent := range hostVersions {
		err = errorlist.Append(err, agent.err)

		if hubVersion != agent.version {
			mismatched[agent.version] = append(mismatched[agent.version], agent.host)
		}
	}

	if err != nil {
		return err
	}

	if len(mismatched) == 0 {
		return nil
	}

	return xerrors.Errorf(`gpupgrade version mismatch between gpupgrade hub and agent hosts. 
    Hub version: %q

    Mismatched Agents:
    %s`, hubVersion, mismatched)
}

type MismatchedVersions map[string][]string

func (m MismatchedVersions) String() string {
	var text string
	for version, hosts := range m {
		sort.Strings(hosts)
		text += fmt.Sprintf("%q: %s\n", version, strings.Join(hosts, ", "))
	}
	return text
}
