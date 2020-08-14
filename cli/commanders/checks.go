// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func RunChecks(client idl.CliToHubClient, ratio float64) error {
	return CheckDiskSpace(client, ratio)
}

type DiskSpaceError struct {
	Failed disk.SpaceFailures
}

func FormatBytes(kb uint64) string {
	bytes := float64(kb)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	for _, unit := range units {
		if bytes < 1024.0 {
			return fmt.Sprintf("%.4g %s", bytes, unit)
		}
		bytes /= 1024.0
	}
	return fmt.Sprintf("%.4g %s", bytes, "EiB")
}

func (d DiskSpaceError) Error() string {
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

func (d DiskSpaceError) Table() [][]string {
	var rows [][]string

	for id, disk := range d.Failed {
		parts := strings.Split(id, ": ")
		host, fs := parts[0], parts[1]

		available := FormatBytes(disk.Available)
		required := FormatBytes(disk.Required)
		needed := FormatBytes(disk.Required - disk.Available)

		rows = append(rows, []string{host, fs, needed, available, required})
	}

	sort.Sort(tableRows(rows))
	rows = append([][]string{{"Hostname", "Filesystem", "Shortfall", "Available", "Required"}}, rows...)

	return rows
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

func CheckDiskSpace(client idl.CliToHubClient, ratio float64) (err error) {
	reply, err := client.CheckDiskSpace(context.Background(), &idl.CheckDiskSpaceRequest{Ratio: ratio})
	if err != nil {
		return xerrors.Errorf("check disk space: %w", err)
	}
	if len(reply.Failed) > 0 {
		return DiskSpaceError{reply.Failed}
	}
	return nil
}
