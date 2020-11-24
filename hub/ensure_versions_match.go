// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

type HubAndAgentVersions interface {
	HubVersion() (string, error)
	AgentVersion(host string) (string, error)
}

type HostVersion struct {
	host         string
	agentVersion string
	err          error
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

func EnsureVersionsMatch(agentHosts []string, version HubAndAgentVersions) error {
	hubVersion, err := version.HubVersion()
	if err != nil {
		return xerrors.Errorf("getting hub version: %w", err)
	}

	var wg sync.WaitGroup
	hostVersions := make(chan HostVersion, len(agentHosts))

	for _, host := range agentHosts {
		wg.Add(1)

		go func(host string) {
			defer wg.Done()

			agentVersion, err := version.AgentVersion(host)
			hostVersions <- HostVersion{host: host, agentVersion: agentVersion, err: err}
		}(host)
	}

	wg.Wait()
	close(hostVersions)

	var errs error
	mismatched := make(MismatchedVersions)
	for hv := range hostVersions {
		errs = errorlist.Append(errs, hv.err)

		if hubVersion != hv.agentVersion {
			mismatched[hv.agentVersion] = append(mismatched[hv.agentVersion], hv.host)
		}
	}

	if errs != nil {
		return errs
	}

	if len(mismatched) == 0 {
		return nil
	}

	return xerrors.Errorf(`Version mismatch between gpupgrade hub and agent hosts. 
    Hub version: %q

    Mismatched Agents:
    %s`, hubVersion, mismatched)
}
