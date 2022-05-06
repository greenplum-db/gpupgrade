// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package greenplum_test

import (
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

func TestSelect(t *testing.T) {
	segs := greenplum.SegConfigs{
		{ContentID: 1, Role: greenplum.PrimaryRole},
		{ContentID: 2, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.PrimaryRole},
		{ContentID: 3, Role: greenplum.MirrorRole},
	}

	// Ensure all segments are visited correctly.
	selectAll := func(_ *greenplum.SegConfig) bool { return true }
	results := segs.Select(selectAll)

	if !reflect.DeepEqual(results, segs) {
		t.Errorf("SelectSegments(*) = %+v, want %+v", results, segs)
	}

	// Test a simple selector.
	moreThanOne := func(c *greenplum.SegConfig) bool { return c.ContentID > 1 }
	results = segs.Select(moreThanOne)

	expected := greenplum.SegConfigs{segs[1], segs[2], segs[3]}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("SelectSegments(ContentID > 1) = %+v, want %+v", results, expected)
	}

}
