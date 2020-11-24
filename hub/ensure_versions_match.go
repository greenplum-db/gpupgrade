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
	host             string
	gpupgradeVersion string
	err              error
}

type MismatchedVersions map[string][]string

func (m MismatchedVersions) String() string {
	var text string
	for version, hosts := range m {
		text += fmt.Sprintf("%q: %s\n", version, strings.Join(hosts, ", "))
	}
	return text
}

func EnsureGpupgradeAndGPDBVersionsMatch(agentHosts []string) error {
	hubGpupgradeVersion, err := GpupgradeVersion()
	if err != nil {
		return xerrors.Errorf("getting hub version: %w", err)
	}

	var wg sync.WaitGroup
	versions := make(chan HostVersion, len(agentHosts))

	for _, host := range agentHosts {
		wg.Add(1)

		go func(host string) {
			defer wg.Done()

			gpupgradeVersion, err := GpupgradeVersionOnHost(host)
			versions <- HostVersion{host: host, gpupgradeVersion: gpupgradeVersion, err: err}
		}(host)
	}

	wg.Wait()
	close(versions)

	var errs error
	mismatchedGpupgradeVersions := make(MismatchedVersions)
	for version := range versions {
		errs = errorlist.Append(errs, version.err)

		if hubGpupgradeVersion != version.gpupgradeVersion {
			mismatchedGpupgradeVersions[version.gpupgradeVersion] = append(mismatchedGpupgradeVersions[version.gpupgradeVersion], version.host)
		}
	}

	if errs != nil {
		return errs
	}

	if len(mismatchedGpupgradeVersions) != 0 {
		return xerrors.Errorf(`Version mismatch between gpupgrade hub and agent hosts. 
Hub version: %q

Mismatched Agents:
%s`, hubGpupgradeVersion, mismatchedGpupgradeVersions)
	}

	return nil
}
