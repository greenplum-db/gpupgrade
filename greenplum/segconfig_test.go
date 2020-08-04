package greenplum_test

import (
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
)

func TestSelect(t *testing.T) {
	segs := greenplum.SegConfigs{
		{ContentID: 1, Role: "p"},
		{ContentID: 2, Role: "p"},
		{ContentID: 3, Role: "p"},
		{ContentID: 3, Role: "m"},
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
