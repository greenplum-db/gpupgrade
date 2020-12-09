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

type ObtainVersions interface {
	Local() (string, error)
	Remote(host string) (string, error)
	Description() string
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

func EnsureVersionsMatch(agentHosts []string, version ObtainVersions) error {
	type agentVersion struct {
		host    string
		version string
		err     error
	}

	hubVersion, err := version.Local()
	if err != nil {
		return xerrors.Errorf("hub version: %w", err)
	}

	agentVersions := make(chan agentVersion, len(agentHosts))
	var wg sync.WaitGroup

	for _, host := range agentHosts {
		host := host

		wg.Add(1)
		go func() {
			defer wg.Done()

			version, err := version.Remote(host)
			agentVersions <- agentVersion{host: host, version: version, err: err}
		}()
	}

	wg.Wait()
	close(agentVersions)

	mismatched := make(MismatchedVersions)
	for agent := range agentVersions {
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

	return xerrors.Errorf(`%s version mismatch between gpupgrade hub and agent hosts. 
    Hub version: %q

    Mismatched Agents:
    %s`, version.Description(), hubVersion, mismatched)
}
