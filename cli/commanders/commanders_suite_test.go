// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"os"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func TestMain(m *testing.M) {
	testhelper.SetupTestLogger()
	os.Exit(exectest.Run(m))
}
