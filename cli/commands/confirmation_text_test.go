// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils"
)

const GPUPGRADE_CONFIG = "../../gpupgrade_config"

func TestInitializeConfirmationText(t *testing.T) {
	t.Run("contains all names defined in the example config file", func(t *testing.T) {
		config := testutils.MustReadFile(t, GPUPGRADE_CONFIG)

		names, err := parseParams(strings.NewReader(config))
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}

		// Since the confirmation text is free-form, there's not much parsing we can do
		// other than make sure "name:" appears in the file somewhere.
		for name := range names {
			if !strings.Contains(initializeConfirmationText, name+":") {
				t.Errorf("expected %q to contain %q", initializeConfirmationText, name)
			}
		}
	})
}
