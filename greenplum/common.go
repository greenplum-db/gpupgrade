// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum

import (
	"testing"
)

func MustCreateCluster(t *testing.T, segments SegConfigs) *Cluster {
	t.Helper()

	cluster, err := NewCluster(segments)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	return &cluster
}
