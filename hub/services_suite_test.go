// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

// Enable exectest.NewCommand mocking.
func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
