// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"fmt"
	"sort"
)

var scriptDescription = map[string]string{
	"cluster_stats":                         "Generates cluster statistics such as number of segments",
	"database_stats":                        "Generates database statistics such as number of indexes and tables",
	"gphdfs_external_tables":                "Drops gphdfs external tables",
	"gphdfs_user_roles":                     "Alters gphdfs user role to not create external tables",
	"heterogeneous_partitioned_tables":      "Ensures child partitions have the same on-disk layout as their root",
	"parent_partitions_with_seg_entries":    "Fixes non-empty segment relfiles for AO and AOCO parent partitions",
	"partitioned_tables_indexes":            "Drops partition indexes",
	"tables_using_name_and_tsquery":         "Alters NAME and TSQUERY column types to VARCHAR",
	"unique_primary_foreign_key_constraint": "Drops constraints",
}

type Script struct {
	Num  uint64
	Name string
}

type Scripts []Script

func (scripts Scripts) Find(num uint64) Script {
	for _, script := range scripts {
		if script.Num == num {
			return script
		}
	}

	return Script{}
}

func (scripts Scripts) Names() []string {
	var names []string
	for _, script := range scripts {
		names = append(names, script.Name)
	}

	return names
}

func (scripts Scripts) Len() int {
	return len(scripts)
}

func (scripts Scripts) Less(i, j int) bool {
	return scripts[i].Num < scripts[j].Num
}

func (scripts Scripts) Swap(i, j int) {
	scripts[i], scripts[j] = scripts[j], scripts[i]
}

func (scripts Scripts) String() string {
	sort.Sort(scripts)

	var output string
	for _, script := range scripts {
		output += fmt.Sprintf("  %d: %s\n", script.Num, script.Name)
	}
	return output
}

func (scripts Scripts) Description() string {
	sort.Sort(scripts)

	var output string
	for _, script := range scripts {
		output += fmt.Sprintf("  %d: %s\n     %s\n\n", script.Num, script.Name, scriptDescription[script.Name])
	}
	return output
}
