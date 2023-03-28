// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package backupdir

type BackupDirs struct {
	// Can collapse the CoordinatorBackupDir into AgentHostsToBackupDir once we run an agent on the coordinator
	// to simplify various logic.
	CoordinatorBackupDir  string
	AgentHostsToBackupDir AgentHostsToBackupDir
}

type AgentHostsToBackupDir map[string]string
