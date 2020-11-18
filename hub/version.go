// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"sort"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var GetVersionFunc = GetVersion

type agentVersion struct {
	host    string
	version string
	err     error
}

type agents map[string]string

func (a agents) String() string {
	hosts := make([]string, 0, len(a))
	for h := range a {
		hosts = append(hosts, h)
	}

	s := ""
	sort.Strings(hosts)
	for _, k := range hosts {
		s += fmt.Sprintf("%s: %s\n", k, a[k])
	}
	return s
}

func ValidateGpupgradeVersion(hubHost string, agentHosts []string) error {
	gpupgradePath, err := utils.GetGpupgradePath()
	if err != nil {
		return xerrors.Errorf("getting gpupgrade binary path: %w", err)
	}

	hubVersion, err := GetVersionFunc(hubHost, gpupgradePath)
	if err != nil {
		return xerrors.Errorf("getting hub version: %w", err)
	}

	var wg sync.WaitGroup
	agentChan := make(chan agentVersion, len(agentHosts))

	for _, host := range agentHosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			version, err := GetVersionFunc(host, gpupgradePath)
			agentChan <- agentVersion{host, version, err}
		}(host)
	}

	wg.Wait()
	close(agentChan)

	var errs error
	mismatchedAgents := make(agents)
	for agent := range agentChan {
		errs = errorlist.Append(errs, agent.err)
		if hubVersion != agent.version {
			mismatchedAgents[agent.host] = agent.version
		}
	}

	if errs != nil {
		return errs
	}

	if len(mismatchedAgents) == 0 {
		return nil
	}

	return xerrors.Errorf(`Version mismatch between gpupgrade hub and agent hosts. 
Hub version: %s

Mismatched Agents:
%s`, hubVersion, mismatchedAgents.String())
}

func GetVersion(host, path string) (string, error) {
	cmd := execCommand("ssh", host, fmt.Sprintf(`bash -c "%s version"`, path))
	gplog.Debug("running cmd %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", xerrors.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	gplog.Debug("output: %q", output)

	return string(output), nil
}
