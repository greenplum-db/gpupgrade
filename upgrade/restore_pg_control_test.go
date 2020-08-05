// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package upgrade_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/upgrade"
	"github.com/greenplum-db/gpupgrade/utils"
)

// Common setup required for multiple test cases
func setupGlobalDir(t *testing.T) string {
	source := testutils.GetTempDir(t, "")

	globalDir := filepath.Join(source, "global")
	err := os.Mkdir(globalDir, 0755)
	if err != nil {
		t.Fatalf("failed to create dir %s", globalDir)
	}

	return source
}

func TestRestorePgControl(t *testing.T) {
	testhelper.SetupTestLogger()

	t.Run("restores pg_control successfully", func(t *testing.T) {
		var buf bytes.Buffer
		outStream := testutils.DevNullSpy{
			OutStream: &buf,
		}

		source := setupGlobalDir(t)
		defer testutils.MustRemoveAll(t, source)

		src := filepath.Join(source, "global", "pg_control.old")
		testutils.MustWriteToFile(t, src, "")

		err := upgrade.RestorePgControl(source, outStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		dst := filepath.Join(source, "global", "pg_control")
		expected, _ := regexp.Compile(fmt.Sprintf("renaming %q to %q", src, dst))
		actual := buf.String()
		if !expected.MatchString(actual) {
			t.Errorf("got stream output %s want %s", actual, expected)
		}

		_, err = os.Stat(dst)
		if err != nil {
			t.Errorf("expected file %s to exist, got error %#v", dst, err)
		}
	})

	t.Run("re-run of RestorePgControl finishes successfully if RestorePgControl already succeeded before", func(t *testing.T) {
		source := setupGlobalDir(t)
		defer testutils.MustRemoveAll(t, source)

		src := filepath.Join(source, "global", "pg_control.old")
		testutils.MustWriteToFile(t, src, "")

		err := upgrade.RestorePgControl(source, step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		dst := filepath.Join(source, "global", "pg_control")
		_, err = os.Stat(dst)
		if err != nil {
			t.Errorf("expected file %s to exist, got error %#v", dst, err)
		}

		err = upgrade.RestorePgControl(source, step.DevNullStream)
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		_, err = os.Stat(dst)
		if err != nil {
			t.Errorf("expected file %s to exist, got error %#v", dst, err)
		}
	})

	t.Run("should fail when pg_control.old and pg_control does not exist", func(t *testing.T) {
		source := setupGlobalDir(t)
		defer testutils.MustRemoveAll(t, source)

		err := upgrade.RestorePgControl(source, step.DevNullStream)
		if err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("should fail if src file exist but stat resulted in an error", func(t *testing.T) {
		source := setupGlobalDir(t)
		defer testutils.MustRemoveAll(t, source)

		src := filepath.Join(source, "global", "pg_control.old")
		expectedError := errors.New("permission denied")
		utils.System.Stat = func(path string) (os.FileInfo, error) {
			if path != src {
				t.Errorf("got path %q, want %q", path, src)
			}

			return nil, expectedError
		}
		defer func() {
			utils.System = utils.InitializeSystemFunctions()
		}()

		err := upgrade.RestorePgControl(source, step.DevNullStream)
		if !errors.Is(err, expectedError) {
			t.Errorf("got error type %T, want %T", err, expectedError)
		}
	})
}
