// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

// RestorePgControl renames pg_control.old to pg_control so that
// the coordinator and segments can start after pg_upgrade is run in
// link mode.
// The dir passed to RestorePgControl must be a data directory.
// It is idempotent as it already checks if the rename has occurred.
func RestorePgControl(dataDir string, streams step.OutStreams) error {
	globalDir := filepath.Join(dataDir, "global")
	src := filepath.Join(globalDir, "pg_control.old")
	dst := filepath.Join(globalDir, "pg_control")

	renamed, err := AlreadyRenamed(src, dst)
	if err != nil {
		return err
	}

	if renamed {
		log.Printf("already renamed %q to %q", src, dst)
		_, err = fmt.Fprintf(streams.Stdout(), "already renamed %q to %q", src, dst)
		if err != nil {
			return err
		}

		return nil
	}

	log.Printf("renaming %q to %q", src, dst)
	_, err = fmt.Fprintf(streams.Stdout(), "renaming %q to %q", src, dst)
	if err != nil {
		return err
	}

	return utils.System.Rename(src, dst)
}
