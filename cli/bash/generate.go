// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

// This binary exists purely for the purpose of generating bash completion for
// the CLI.  You can run `go generate ./cli/bash` to regen the
// bash-completion.sh script.
package main

import (
	"log"
	"os"

	"github.com/greenplum-db/gpupgrade/cli/commands"
)

//go:generate go run generate.go bash-completion.sh

func main() {
	root := commands.BuildRootCommand()
	err := root.GenBashCompletionFile(os.Args[1])
	if err != nil {
		log.Fatalf("generating bash-completion.sh: %+v", err)
	}
}
