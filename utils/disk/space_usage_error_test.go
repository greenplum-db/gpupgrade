// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package disk_test

import (
	"github.com/greenplum-db/gpupgrade/idl"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes    uint64
		expected string
	}{
		// Testing units
		{42, "42 KB"},
		{42 * 1000, "42 MB"},
		{42 * 1000 * 1000, "42 GB"},
		{42 * 1000 * 1000 * 1000, "42 TB"},
		{42 * 1000 * 1000 * 1000 * 1000, "42 PB"},
		{42 * 1000 * 1000 * 1000 * 1000 * 1000, "42 EB"},
		// Testing rounding
		{0, "0 KB"},
		{4200, "4.2 MB"},
		{4291, "4.291 MB"},
		{4300, "4.3 MB"},
		{12636, "12.64 MB"},
		{126362, "126.4 MB"},
		{1048064, "1.048 GB"},
	}
	for _, c := range cases {
		actual := disk.FormatBytes(c.bytes)
		if actual != c.expected {
			t.Errorf("FormatBytes(%d)=%q, want %q", c.bytes, actual, c.expected)
		}
	}
}

func TestDiskSpaceError(t *testing.T) {
	err := disk.NewSpaceUsageError(map[disk.FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage {
		disk.FilesystemHost{Filesystem: "/", Host: "sdw1"}: {
			Fs:        "/",
			Host:      "sdw1",
			Available: 15,
			Required:  20,
		},
		disk.FilesystemHost{Filesystem: "/proc", Host: "sdw1"}: {
			Fs:        "/proc",
			Host:      "sdw1",
			Available: 15,
			Required:  20,
		},
		disk.FilesystemHost{Filesystem: "/", Host: "mdw"}: {
			Fs:        "/",
			Host:      "mdw",
			Available: 1024,
			Required:  2048,
		},
	})

	rows := err.Table()

	expected := [][]string{
		{"Hostname", "Filesystem", "Shortfall", "Available", "Required"},
		{"mdw", "/", disk.FormatBytes(1024), disk.FormatBytes(1024), disk.FormatBytes(2048)},
		{"sdw1", "/", disk.FormatBytes(5), disk.FormatBytes(15), disk.FormatBytes(20)},
		{"sdw1", "/proc", disk.FormatBytes(5), disk.FormatBytes(15), disk.FormatBytes(20)},
	}
	if !reflect.DeepEqual(expected, rows) {
		t.Errorf("got table %q, want %q", rows, expected)
	}
}
