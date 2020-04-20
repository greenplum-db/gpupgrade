// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/upgrade"
)

func (s *Server) Revert(_ *idl.RevertRequest, stream idl.CliToHub_RevertServer) (err error) {
	if len(s.Config.Target.Primaries) > 0 {
		err = DeletePrimaryDataDirectories(s.agentConns, s.Config.Target)
		if err != nil {
			return err
		}

		err = upgrade.DeleteDirectories([]string{s.Config.Target.MasterDataDir()}, upgrade.PostgresFiles)
		if err != nil {
			return err
		}
	}

	return DeleteStateDirectories(s.agentConns, s.Source.MasterHostname())
}
