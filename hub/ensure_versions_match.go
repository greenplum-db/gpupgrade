// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var GpupgradeVersion = upgrade.GpupgradeVersion
var GpupgradeVersionOnHost = upgrade.GpupgradeVersionOnHost

type HostVersion struct {
	host    string
	version string
	err     error
}

type MismatchedVersions map[string][]string

func (m MismatchedVersions) String() string {
	var text string
	for version, hosts := range m {
		text += fmt.Sprintf("%q: %s\n", version, strings.Join(hosts, ", "))
	}
	return text
}

func EnsureGpupgradeVersionsMatch(agentHosts []string) error {
	hubGpupgradeVersion, err := GpupgradeVersion()
	if err != nil {
		return xerrors.Errorf("getting hub version: %w", err)
	}

	mismatchedVersions, err := ensureVersionsMatch(agentHosts, hubGpupgradeVersion, GpupgradeVersionOnHost)
	if err != nil {
		return err
	}

	if len(mismatchedVersions) == 0 {
		return nil
	}

	return xerrors.Errorf(`Version mismatch between gpupgrade hub and agent hosts. 
Hub version: %q

Mismatched Agents:
%s`, hubGpupgradeVersion, mismatchedVersions)
}

func ensureVersionsMatch(agentHosts []string, hubVersion string, getVersion func(string) (string, error)) (MismatchedVersions, error) {
	var wg sync.WaitGroup
	hostVersions := make(chan HostVersion, len(agentHosts))

	for _, host := range agentHosts {
		wg.Add(1)

		go func(host string) {
			defer wg.Done()

			gpupgradeVersion, err := getVersion(host)
			hostVersions <- HostVersion{host: host, version: gpupgradeVersion, err: err}
		}(host)
	}

	wg.Wait()
	close(hostVersions)

	var errs error
	mismatchedVersions := make(MismatchedVersions)
	for hostVersion := range hostVersions {
		errs = errorlist.Append(errs, hostVersion.err)

		if hubVersion != hostVersion.version {
			mismatchedVersions[hostVersion.version] = append(mismatchedVersions[hostVersion.version], hostVersion.host)
		}
	}

	if errs != nil {
		return MismatchedVersions{}, errs
	}

	return mismatchedVersions, nil
}
