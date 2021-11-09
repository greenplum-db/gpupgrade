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

func AppendDynamicLibraryPath(intermediate *greenplum.Cluster, dynamicLibraryPath string) error {
	stream := &step.BufferedStreams{}

	// get current dynamic_library_path
	args := []string{"-s", "dynamic_library_path"}
	err := intermediate.RunGreenplumCmdWithEnvironment(stream, "gpconfig", args, utils.FilterEnv([]string{"USER"})) // gpconfig requires the USER environment variable
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

	// append dynamic_library_path to current value
	args = []string{"-c", "dynamic_library_path", "-v", currentValue + ":" + dynamicLibraryPath}
	err = intermediate.RunGreenplumCmdWithEnvironment(stream, "gpconfig", args, utils.FilterEnv([]string{"USER"})) // gpconfig requires the USER environment variable
	if err != nil {
		return err
	}

	return intermediate.RunGreenplumCmd(stream, "gpstop", "-u")
}
