// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/exectest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Services Suite")
}

// Enable exectest.NewCommand mocking.
func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}
