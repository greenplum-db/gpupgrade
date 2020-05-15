// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestHub(t *testing.T) {
	t.Run("the hub does not daemonizes unless --deamonize is passed", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", filepath.Join(dir, ".gpupgrade"))
		defer resetEnv()

		err := commanders.CreateStateDir()
		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		err = commanders.CreateInitialClusterConfigs()
		if err != nil {
			t.Errorf("unexpected error got %+v", err)
		}

		cmd := exec.Command("gpupgrade", "hub")
		err = cmd.Start()
		if err != nil {
			t.Errorf("unexpected error %+v", err)
		}
		defer func() {
			if err := cmd.Process.Kill(); err != nil {
				t.Fatalf("failed to kill gpupgrade hub: %+v", err)
			}
		}()

		errChan := make(chan error, 1)
		go func() {
			errChan <- cmd.Wait() // expected to never return
		}()

		select {
		case err := <-errChan:
			if err != nil {
				t.Errorf("unexpected error %+v", err)
			}
		case <-time.After(100 * time.Millisecond):
			// hub daemonizes without an error
		}
	})
}
