// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package disk_test

import (
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
		{42, "42 KiB"},
		{42 * 1024, "42 MiB"},
		{42 * 1024 * 1024, "42 GiB"},
		{42 * 1024 * 1024 * 1024, "42 TiB"},
		{42 * 1024 * 1024 * 1024 * 1024, "42 PiB"},
		{42 * 1024 * 1024 * 1024 * 1024 * 1024, "42 EiB"},
		// Testing rounding
		{0, "0 KiB"},
		{4200, "4.102 MiB"},
		{4291, "4.19 MiB"},
		{4300, "4.199 MiB"},
		{12636, "12.34 MiB"},
		{126362, "123.4 MiB"},
		{1048064, "1024 MiB"},
	}
	for _, c := range cases {
		actual := disk.FormatBytes(c.bytes)
		if actual != c.expected {
			t.Errorf("FormatBytes(%d)=%q, want %q", c.bytes, actual, c.expected)
		}
	}
}

func TestDiskSpaceError(t *testing.T) {
	err := disk.NewSpaceUsageError(disk.FileSystemDiskUsage{
		{
			Fs:        "/",
			Host:      "sdw1",
			Available: 15,
			Required:  20,
		},
		{
			Fs:        "/proc",
			Host:      "sdw1",
			Available: 15,
			Required:  20,
		},
		{
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
