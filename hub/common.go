// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import "os/exec"

//
// Override internals of the hub package
//

// Allow exec.Command to be mocked out by exectest.NewCommand.
var ExecCommand = exec.Command
