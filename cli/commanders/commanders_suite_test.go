// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
)

func TestMain(m *testing.M) {
	testlog.SetupLogger()
	os.Exit(exectest.Run(m))
}
