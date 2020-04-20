// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package agent

func SetDeleteDirectories(mockDeleteDirectories func([]string, []string) error) func() {
	original := deleteDirectories
	deleteDirectories = mockDeleteDirectories
	return func() {
		deleteDirectories = original
	}
}
