// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package integrations_test

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func init() {
	// All gpupgrade binaries are expected to be on the path for integration
	// tests. Be nice to developers and check up front; warn if the binaries
	// being tested aren't contained in a directory directly above this test
	// file.
	_, testPath, _, ok := runtime.Caller(0)
	if !ok {
		panic("couldn't retrieve Caller() information")
	}

	var allErrs error
	for _, bin := range []string{"gpupgrade"} {
		binPath, err := exec.LookPath(bin)
		if err != nil {
			allErrs = errorlist.Append(allErrs, err)
			continue
		}

		dir := filepath.Dir(binPath)
		if !strings.HasPrefix(testPath, dir) {
			log.Printf("warning: tested binary %s doesn't appear to be locally built", binPath)
		}
	}
	if allErrs != nil {
		panic(fmt.Sprintf(
			"Please put gpupgrade binaries on your PATH before running integration tests.\n%s",
			allErrs,
		))
	}
}
