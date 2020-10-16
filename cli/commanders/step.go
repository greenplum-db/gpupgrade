//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/cli"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

const StepsFileName = "steps.json"

type CLIStep struct {
	stepName    string
	step        idl.Step
	store       *StepStore
	streams     *step.BufferedStreams
	verbose     bool
	timer       *stopwatch.Stopwatch
	lastSubstep idl.Substep
	err         error
}

func NewStep(step idl.Step, streams *step.BufferedStreams, verbose bool) (*CLIStep, error) {
	store, err := NewStepStore()
	if err != nil {
		return &CLIStep{}, err
	}

	err = store.Write(step, idl.Status_RUNNING)
	if err != nil {
		return &CLIStep{}, err
	}

	stepName := strings.Title(strings.ToLower(step.String()))

	fmt.Println()
	fmt.Println(stepName + " in progress.")
	fmt.Println()

	return &CLIStep{
		stepName: stepName,
		step:     step,
		store:    store,
		streams:  streams,
		verbose:  verbose,
		timer:    stopwatch.Start(),
	}, nil
}

func (s *CLIStep) Err() error {
	return s.err
}

func (s *CLIStep) RunHubSubstep(f func(streams step.OutStreams) error) {
	if s.err != nil {
		return
	}

	err := f(s.streams)
	if err != nil {
		if errors.Is(err, step.Skip) {
			return
		}

		s.err = err
	}
}

func (s *CLIStep) RunInternalSubstep(f func() error) {
	if s.err != nil {
		return
	}

	err := f()
	if err != nil {
		if errors.Is(err, step.Skip) {
			return
		}

		s.err = err
	}
}

func (s *CLIStep) RunCLISubstep(substep idl.Substep, f func(streams step.OutStreams) error) {
	var err error
	defer func() {
		if err != nil {
			s.err = xerrors.Errorf("substep %q: %w", substep, err)
		}
	}()

	if s.err != nil {
		return
	}

	substepTimer := stopwatch.Start()
	defer func() {
		logDuration(substep.String(), s.verbose, substepTimer.Stop())
	}()

	s.printStatus(substep, idl.Status_RUNNING)

	err = f(s.streams)
	if s.verbose {
		fmt.Println() // Reset the cursor so verbose output does not run into the status.

		_, wErr := s.streams.StdoutBuf.WriteTo(os.Stdout)
		if wErr != nil {
			err = errorlist.Append(err, xerrors.Errorf("writing stdout: %w", wErr))
		}

		_, wErr = s.streams.StderrBuf.WriteTo(os.Stderr)
		if wErr != nil {
			err = errorlist.Append(err, xerrors.Errorf("writing stderr: %w", wErr))
		}
	}

	if err != nil {
		status := idl.Status_FAILED

		if errors.Is(err, step.Skip) {
			status = idl.Status_SKIPPED
			err = nil
		}

		s.printStatus(substep, status)
		return
	}

	s.printStatus(substep, idl.Status_COMPLETE)
}

func (s *CLIStep) DisableStore() {
	s.store = nil
}

func (s *CLIStep) Complete(completedText string) error {
	logDuration(s.stepName, s.verbose, s.timer.Stop())

	status := idl.Status_COMPLETE
	if s.Err() != nil {
		status = idl.Status_FAILED
	}

	if s.store != nil {
		if wErr := s.store.Write(s.step, status); wErr != nil {
			s.err = errorlist.Append(s.err, wErr)
		}
	}

	if s.Err() != nil {
		fmt.Println()

		// allow substpes to override the default next actions
		var nextActions cli.NextActions
		if errors.As(s.Err(), &nextActions) {
			return nextActions
		}

		msg := fmt.Sprintf(`Please address the above issue and run "gpupgrade %s" again.
If you would like to return the cluster to its original state, please run "gpupgrade revert".`, strings.ToLower(s.stepName))
		return cli.NewNextActions(s.Err(), msg)
	}

	fmt.Println(completedText)
	return nil
}

func (s *CLIStep) printStatus(substep idl.Substep, status idl.Status) {
	if substep == s.lastSubstep {
		// For the same substep reset the cursor to overwrite the current status.
		fmt.Print("\r")
	}

	text := SubstepDescriptions[substep]
	fmt.Print(Format(text.OutputText, status))

	// Reset the cursor if the final status has been written. This prevents the
	// status from a hub step from being on the same line as a CLI step.
	if status != idl.Status_RUNNING {
		fmt.Println()
	}

	s.lastSubstep = substep
}

func logDuration(operation string, verbose bool, timer *stopwatch.Stopwatch) {
	msg := operation + " took " + timer.String()
	if verbose {
		fmt.Println(msg)
		fmt.Println()
	}
	gplog.Debug(msg)
}
