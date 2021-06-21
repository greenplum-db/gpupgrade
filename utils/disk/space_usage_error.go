// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/greenplum-db/gpupgrade/idl"
)

type SpaceUsageErr struct {
	usage FileSystemDiskUsage
}

func NewSpaceUsageError(usageMap map[FilesystemHost]*idl.CheckDiskSpaceReply_DiskUsage) *SpaceUsageErr {
	var totalUsage FileSystemDiskUsage

	for _, usage := range usageMap {
		totalUsage = append(totalUsage, usage)
	}

	return &SpaceUsageErr{usage: totalUsage}
}

func NewSpaceUsageErrorFromUsage(usage idl.CheckDiskSpaceReply_DiskUsage) *SpaceUsageErr {
	return &SpaceUsageErr{usage: FileSystemDiskUsage{&usage}}
}

func (d SpaceUsageErr) Error() string {
	var b strings.Builder
	b.WriteString("You currently do not have enough disk space to run an upgrade.\n\n")

	// Pretty-print our output with tab-alignment.
	var t tabwriter.Writer
	t.Init(&b, 0, 0, 2, ' ', 0)

	for _, row := range d.Table() {
		for _, col := range row {
			fmt.Fprintf(&t, "%s\t", col)
		}
		fmt.Fprintln(&t)
	}

	t.Flush()
	return b.String()
}

func (d SpaceUsageErr) Table() [][]string {
	var rows [][]string

	for _, usage := range d.usage {
		available := FormatBytes(usage.GetAvailable())
		required := FormatBytes(usage.GetRequired())
		needed := FormatBytes(usage.GetRequired() - usage.GetAvailable())

		rows = append(rows, []string{usage.GetHost(), usage.GetFs(), needed, available, required})
	}

	sort.Sort(tableRows(rows))
	rows = append([][]string{{"Hostname", "Filesystem", "Shortfall", "Available", "Required"}}, rows...)

	return rows
}

func FormatBytes(kb uint64) string {
	bytes := float64(kb)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for _, unit := range units {
		if bytes < 1000.0 {
			return fmt.Sprintf("%.4g %s", bytes, unit)
		}
		bytes /= 1000.0
	}
	return fmt.Sprintf("%.4g %s", bytes, "EB")
}

// tableRows attaches sort.Interface to a slice of string slices.
type tableRows [][]string

func (t tableRows) Len() int {
	return len(t)
}

func (t tableRows) Less(i, j int) bool {
	ri, rj := t[i], t[j]

	// Sort by hostname, then by filesystem.
	if ri[0] == rj[0] {
		return ri[1] < rj[1]
	}
	return ri[0] < rj[0]
}

func (t tableRows) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}
