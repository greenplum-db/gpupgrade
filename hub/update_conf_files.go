// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/blang/semver/v4"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func UpdateConfFiles(agentConns []*idl.Connection, _ step.OutStreams, version semver.Version, intermediate *greenplum.Cluster, target *greenplum.Cluster) error {
	if version.Major < 7 {
		if err := UpdateGpperfmonConf(target.MasterDataDir()); err != nil {
			return err
		}
	}

	opt := []*idl.UpdateFileConfOptions{{
		Path:         filepath.Join(target.MasterDataDir(), "postgresql.conf"),
		CurrentValue: int32(intermediate.MasterPort()),
		UpdatedValue: int32(target.MasterPort()),
	}}
	if err := UpdatePostgresqlConf(opt); err != nil {
		return err
	}

	if err := UpdatePostgresqlConfOnSegments(agentConns, intermediate, target); err != nil {
		return err
	}

	if err := UpdateRecoveryConfiguration(agentConns, version, intermediate, target); err != nil {
		return err
	}

	return nil
}

func UpdatePostgresqlConfOnSegments(agentConns []*idl.Connection, intermediate *greenplum.Cluster, target *greenplum.Cluster) error {
	request := func(conn *idl.Connection) error {
		var opts []*idl.UpdateFileConfOptions

		// add standby
		if target.Standby().Hostname == conn.Hostname {
			opt := &idl.UpdateFileConfOptions{
				Path:         filepath.Join(target.StandbyDataDir(), "postgresql.conf"),
				CurrentValue: int32(intermediate.StandbyPort()),
				UpdatedValue: int32(target.StandbyPort()),
			}

			opts = append(opts, opt)
		}

		// add mirrors
		mirrors := target.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && seg.IsMirror()
		})

		for _, mirror := range mirrors {
			opt := &idl.UpdateFileConfOptions{
				Path:         filepath.Join(mirror.DataDir, "postgresql.conf"),
				CurrentValue: int32(intermediate.Primaries[mirror.ContentID].Port),
				UpdatedValue: int32(mirror.Port),
			}

			opts = append(opts, opt)
		}

		// add primaries
		primaries := target.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && seg.IsPrimary()
		})

		for _, primary := range primaries {
			opt := &idl.UpdateFileConfOptions{
				Path:         filepath.Join(primary.DataDir, "postgresql.conf"),
				CurrentValue: int32(intermediate.Primaries[primary.ContentID].Port),
				UpdatedValue: int32(primary.Port),
			}

			opts = append(opts, opt)
		}

		req := &idl.UpdatePostgresqlConfRequest{Options: opts}
		_, err := conn.AgentClient.UpdatePostgresqlConf(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func UpdateRecoveryConfiguration(agentConns []*idl.Connection, version semver.Version, intermediateCluster *greenplum.Cluster, target *greenplum.Cluster) error {
	file := "postgresql.auto.conf"
	if version.Major == 6 {
		file = "recovery.conf"
	}

	request := func(conn *idl.Connection) error {
		var opts []*idl.UpdateFileConfOptions

		// add standby
		if target.Standby().Hostname == conn.Hostname {
			opt := &idl.UpdateFileConfOptions{
				Path:         filepath.Join(target.StandbyDataDir(), file),
				CurrentValue: int32(intermediateCluster.MasterPort()),
				UpdatedValue: int32(target.MasterPort()),
			}

			opts = append(opts, opt)
		}

		// add mirrors
		mirrors := target.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && seg.IsMirror()
		})

		for _, mirror := range mirrors {
			opt := &idl.UpdateFileConfOptions{
				Path:         filepath.Join(mirror.DataDir, file),
				CurrentValue: int32(intermediateCluster.Primaries[mirror.ContentID].Port),
				UpdatedValue: int32(target.Primaries[mirror.ContentID].Port),
			}

			opts = append(opts, opt)
		}

		req := &idl.UpdateRecoveryConfRequest{Options: opts}
		_, err := conn.AgentClient.UpdateRecoveryConf(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

func UpdateGpperfmonConf(masterDataDir string) error {
	path := filepath.Join(masterDataDir, "gpperfmon", "conf", "gpperfmon.conf")
	pattern := `^log_location = .*$` // TODO: allow arbitrary whitespace around the = sign?
	replacement := fmt.Sprintf("log_location = %s", filepath.Join(masterDataDir, "gpperfmon", "logs"))

	return updateConfigurationFile(path, pattern, replacement)
}

func UpdatePostgresqlConf(opts []*idl.UpdateFileConfOptions) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(opts))

	for _, opt := range opts {

		wg.Add(1)
		go func(opt *idl.UpdateFileConfOptions) {
			defer wg.Done()

			pattern := fmt.Sprintf(`(^port[ \t]*=[ \t]*)%d([^0-9]|$)`, opt.GetCurrentValue())
			replacement := fmt.Sprintf(`\1%d\2`, opt.GetUpdatedValue())

			errs <- updateConfigurationFile(opt.GetPath(), pattern, replacement)
		}(opt)
	}

	wg.Wait()
	close(errs)

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}

func UpdateRecoveryConf(opts []*idl.UpdateFileConfOptions) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(opts))

	for _, opt := range opts {

		wg.Add(1)
		go func(opt *idl.UpdateFileConfOptions) {
			defer wg.Done()

			pattern := fmt.Sprintf(`(primary_conninfo .* port[ \t]*=[ \t]*)%d([^0-9]|$)`, opt.GetCurrentValue())
			replacement := fmt.Sprintf(`\1%d\2`, opt.GetUpdatedValue())

			errs <- updateConfigurationFile(opt.GetPath(), pattern, replacement)
		}(opt)
	}

	wg.Wait()
	close(errs)

	var err error
	for e := range errs {
		err = errorlist.Append(err, e)
	}

	return err
}

func updateConfigurationFile(path string, pattern string, replacement string) error {
	cmd := cmd("sed", "-E", "-i.bak",
		fmt.Sprintf(`s@%s@%s@`, pattern, replacement),
		path,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return xerrors.Errorf("update %s using %q failed with %q: %w", filepath.Base(path), cmd.String(), string(output), err)
	}

	return nil
}
