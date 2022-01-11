//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package hub

import (
	"bufio"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func AppendDynamicLibraryPath(intermediate *greenplum.Cluster, toAppend string) error {
	stream := &step.BufferedStreams{}

	// get current dynamic_library_path from the intermediate target cluster
	err := intermediate.RunGreenplumCmdWithEnvironment(stream,
		"gpconfig", []string{"-s", "dynamic_library_path"},
		utils.FilterEnv([]string{"USER"})) // gpconfig requires the USER environment variable
	if err != nil {
		return err
	}

	var currentValue string
	prefix := "Master  value:"
	scanner := bufio.NewScanner(strings.NewReader(stream.StdoutBuf.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimPrefix(line, prefix)
			currentValue = strings.TrimSpace(line)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return xerrors.Errorf("scanning gpconfig dynamic_library_path output: %w", err)
	}

	if currentValue == "" {
		return fmt.Errorf(`Target cluster is missing value for dynamic_library_path. Expected "$libdir", but found %q: %w`, stream.StdoutBuf.String(), err)
	}

	// append the user specified dynamic_library_path to the current value and remove any duplicates
	dynamicLibraryPath := utils.RemoveDuplicates(append(
		strings.Split(currentValue, ":"),
		strings.Split(toAppend, ":")...))

	// set the dynamic_library_path
	err = intermediate.RunGreenplumCmdWithEnvironment(stream,
		"gpconfig",
		[]string{"-c", "dynamic_library_path", "-v", strings.Join(dynamicLibraryPath, ":")},
		utils.FilterEnv([]string{"USER"})) // gpconfig requires the USER environment variable
	if err != nil {
		return err
	}

	return intermediate.RunGreenplumCmd(stream, "gpstop", "-u")
}
