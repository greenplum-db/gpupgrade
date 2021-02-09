// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

type UpgradeChecker interface {
	UpgradeMaster(args UpgradeMasterArgs) error
	UpgradePrimaries(args UpgradePrimaryArgs) error
}

type upgradeChecker struct{}

func (upgradeChecker) UpgradeMaster(args UpgradeMasterArgs) error {
	return UpgradeMaster(args)
}

func (upgradeChecker) UpgradePrimaries(args UpgradePrimaryArgs) error {
	return UpgradePrimaries(args)
}

var upgrader UpgradeChecker = upgradeChecker{}

func (s *Server) CheckUpgrade(stream step.OutStreams, conns []*Connection) error {
	var wg sync.WaitGroup
	checkErrs := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		checkErrs <- upgrader.UpgradeMaster(UpgradeMasterArgs{
			Source:      s.Source,
			Target:      s.Target,
			StateDir:    s.StateDir,
			Stream:      stream,
			CheckOnly:   true,
			UseLinkMode: s.UseLinkMode,
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		dataDirPairMap, dataDirPairsErr := s.GetDataDirPairs()
		if dataDirPairsErr != nil {
			checkErrs <- xerrors.Errorf("get source and target primary data directories: %w", dataDirPairsErr)
			return
		}

		checkErrs <- upgrader.UpgradePrimaries(UpgradePrimaryArgs{
			CheckOnly:       true,
			MasterBackupDir: "",
			AgentConns:      conns,
			DataDirPairMap:  dataDirPairMap,
			Source:          s.Source,
			Target:          s.Target,
			UseLinkMode:     s.UseLinkMode,
		})
	}()

	wg.Wait()
	close(checkErrs)

	var err error
	for e := range checkErrs {
		err = errorlist.Append(err, e)
	}

	return err
}
