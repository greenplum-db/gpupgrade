// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

func Init5x() {
	ReplacementFuncs = []ReplacementFunc{
		ReplacePrecision,
		Replacements5X,
	}
}
