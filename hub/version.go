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

var GetGpupgradeVersionFunc = GetGpupgradeVersion

type agentVersion struct {
	host             string
	gpupgradeVersion string
	err              error
}

type hostToGpupgradeVersion map[string]string

func (a hostToGpupgradeVersion) String() string {
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

func EnsureGpupgradeAndGPDBVersionsMatch(agentHosts []string, hubHost string) error {
	gpupgradePath, err := utils.GetGpupgradePath()
	if err != nil {
		return xerrors.Errorf("getting gpupgrade binary path: %w", err)
	}

	hubGpupgradeVersion, err := GetGpupgradeVersionFunc(hubHost, gpupgradePath)
	if err != nil {
		return xerrors.Errorf("getting hub version: %w", err)
	}

	var wg sync.WaitGroup
	agentChan := make(chan agentVersion, len(agentHosts))

	for _, host := range agentHosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			gpupgradeVersion, err := GetGpupgradeVersionFunc(host, gpupgradePath)
			agentChan <- agentVersion{host: host, gpupgradeVersion: gpupgradeVersion, err: err}
		}(host)
	}

	wg.Wait()
	close(agentChan)

	var errs error
	mismatchedGpupgradeVersions := make(hostToGpupgradeVersion)
	for agent := range agentChan {
		errs = errorlist.Append(errs, agent.err)

		if hubGpupgradeVersion != agent.gpupgradeVersion {
			mismatchedGpupgradeVersions[agent.host] = agent.gpupgradeVersion
		}
	}

	if errs != nil {
		return errs
	}

	if len(mismatchedGpupgradeVersions) == 0 {
		return nil
	}

	return xerrors.Errorf(`Version mismatch between gpupgrade hub and agent hosts. 
Hub version: %s

Mismatched Agents:
%s`, hubGpupgradeVersion, mismatchedGpupgradeVersions.String())
}

func GetGpupgradeVersion(host, path string) (string, error) {
	cmd := execCommand("ssh", host, fmt.Sprintf(`bash -c "%s version"`, path))
	gplog.Debug("running cmd %q", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", xerrors.Errorf("%q failed with %q: %w", cmd.String(), string(output), err)
	}

	gplog.Debug("output: %q", output)

	return string(output), nil
}
