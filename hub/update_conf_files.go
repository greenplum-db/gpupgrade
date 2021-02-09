// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"fmt"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/step"
)

func UpdateConfFiles(streams step.OutStreams, masterDataDir string, oldPort, newPort int) error {
	if err := UpdateGpperfmonConf(streams, masterDataDir); err != nil {
		return err
	}

	if err := UpdatePostgresqlConf(streams, masterDataDir, oldPort, newPort); err != nil {
		return err
	}

	return nil
}

func UpdateGpperfmonConf(streams step.OutStreams, masterDataDir string) error {
	logDir := filepath.Join(masterDataDir, "gpperfmon", "logs")

	pattern := `^log_location = .*$`
	replacement := fmt.Sprintf("log_location = %s", logDir)

	// TODO: allow arbitrary whitespace around the = sign?
	cmd := execCommand(
		"sed",
		"-i.bak", // in-place substitution with .bak backup extension
		fmt.Sprintf(`s|%s|%s|`, pattern, replacement),
		filepath.Join(masterDataDir, "gpperfmon", "conf", "gpperfmon.conf"),
	)

	cmd.Stdout, cmd.Stderr = streams.Stdout(), streams.Stderr()
	return cmd.Run()
}

func UpdatePostgresqlConf(streams step.OutStreams, dataDir string, oldPort, newPort int) error {
	// NOTE: any additions of forward slashes (/) here require an update to the
	// sed script below
	pattern := fmt.Sprintf(`(^port[ \t]*=[ \t]*)%d([^0-9]|$)`, oldPort)
	replacement := fmt.Sprintf(`\1%d\2`, newPort)

	path := filepath.Join(dataDir, "postgresql.conf")

	cmd := execCommand(
		"sed",
		"-E",     // use POSIX extended regexes
		"-i.bak", // in-place substitution with .bak backup extension
		fmt.Sprintf(`s/%s/%s/`, pattern, replacement),
		path,
	)

	cmd.Stdout, cmd.Stderr = streams.Stdout(), streams.Stderr()
	return cmd.Run()
}
