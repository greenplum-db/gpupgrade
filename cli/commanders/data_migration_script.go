// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"fmt"
	"sort"
)

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
		output += fmt.Sprintf("%d: %s\n", script.Num, script.Name)
	}
	return output
}
