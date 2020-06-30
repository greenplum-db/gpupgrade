// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/greenplum-db/gpupgrade/upgrade/unique"
)

func TestGetArchiveDirectoryName(t *testing.T) {
	// Make sure every part of the date is distinct, to catch mistakes in
	// formatting (e.g. using seconds rather than minutes).
	stamp := time.Date(2000, 03, 14, 12, 15, 45, 1, time.Local)

	var id unique.ID
	actual := GetArchiveDirectoryName(id, stamp)

	expected := fmt.Sprintf("gpupgrade-%s-2000-03-14T12:15", id.String())
	if actual != expected {
		t.Errorf("GetArchiveDirectoryName() = %q, want %q", actual, expected)
	}
}
