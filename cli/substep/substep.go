//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package substep

import (
	"fmt"
	"os"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

// CLISubstep are substeps that are only run on the CLI
type CLISubstep struct {
	streams *step.BufferedStreams // writes stdout/err
	verbose bool
}

func New(streams *step.BufferedStreams, verbose bool) *CLISubstep {
	return &CLISubstep{
		streams: streams,
		verbose: verbose,
	}
}

// Run prints out the corresponding status marker such as running, completed, or
// failed.
func (s *CLISubstep) Run(name idl.Substep, f func(streams step.OutStreams) error) {
	text := commanders.SubstepDescriptions[name]
	var err error
	defer func() {
		if err != nil {
			err = xerrors.Errorf(`substep "%s": %w`, name, err)
		}

		s.finish(&err, name, text, 0)
	}()

	fmt.Printf("%s\r", commanders.Format(text.OutputText, idl.Status_RUNNING))

	err = f(s.streams)
	if s.verbose {
		os.Stdout.Write(s.streams.StdoutBuf.Bytes())
		os.Stdout.Write(s.streams.StderrBuf.Bytes())
	}
}

// finish prints out the final status of the substep; either COMPLETE or FAILED
// depending on whether or not there is an error. The method takes a pointer to
// error rather than error to make it possible to defer:
func (s *CLISubstep) finish(err *error, name idl.Substep, text commanders.SubstepText, duration time.Duration) {
	status := idl.Status_COMPLETE
	if *err != nil {
		status = idl.Status_FAILED
	}

	fmt.Printf("%s\n", commanders.Format(text.OutputText, status))

	durationMsg := fmt.Sprintf("\n%s took %s\n\n", name, duration)
	if s.verbose {
		fmt.Print(durationMsg)
	}

	gplog.Debug(durationMsg)
}
