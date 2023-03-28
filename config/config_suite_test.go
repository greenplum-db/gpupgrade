// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"
)

func PostgresGPVersion_5_29_10() {
	fmt.Println("postgres (Greenplum Database) 5.29.10 build commit:fca0e6aa84a7d611ce8b7986d6fc73ae93b76f5e")
}

func init() {
	exectest.RegisterMains(
		PostgresGPVersion_5_29_10,
	)
}

// Enable exectest.NewCommand mocking.
func TestMain(m *testing.M) {
	os.Exit(exectest.Run(m))
}

// MustCreateCluster creates a utils.Cluster and calls t.Fatalf() if there is
// any error.
func MustCreateCluster(t *testing.T, segments greenplum.SegConfigs) *greenplum.Cluster {
	t.Helper()

	cluster, err := greenplum.NewCluster(segments)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
