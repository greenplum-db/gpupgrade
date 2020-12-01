//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/cli"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

const StepsFileName = "steps.json"

const nextActionRunRevertText = "\nIf you would like to return the cluster to its original state, please run \"gpupgrade revert\"."

var additionalNextActions = map[idl.Step]string{
	idl.Step_INITIALIZE: nextActionRunRevertText,
	idl.Step_EXECUTE:    nextActionRunRevertText,
	idl.Step_FINALIZE:   "",
	idl.Step_REVERT:     "",
}

type Step struct {
	stepName    string
	step        idl.Step
	store       *StepStore
	streams     *step.BufferedStreams
	verbose     bool
	timer       *stopwatch.Stopwatch
	lastSubstep idl.Substep
	err         error
}

func NewStep(currentStep idl.Step, streams *step.BufferedStreams, verbose bool, interactive bool, confirmationText string) (*Step, error) {
	store, err := NewStepStore()
	if err != nil {
		gplog.Error("creating step store: %v", err)
		context := fmt.Sprintf("Note: If commands were issued in order, ensure gpupgrade can write to %s", utils.GetStateDir())
		wrappedErr := xerrors.Errorf("%v\n\n%v", StepErr, context)
		return &Step{}, cli.NewNextActions(wrappedErr, RunInitialize)
	}

	err = store.ValidateStep(currentStep)
	if err != nil {
		return nil, err
	}

	if !interactive {
		fmt.Println(confirmationText)

		proceed, err := Prompt(bufio.NewReader(os.Stdin))
		if err != nil {
			return &Step{}, err
		}

		if !proceed {
			return &Step{}, step.UserCanceled
		}
	}

	err = store.Write(currentStep, idl.Status_RUNNING)
	if err != nil {
		return &Step{}, err
	}

	stepName := strings.Title(strings.ToLower(currentStep.String()))

	fmt.Println()
	fmt.Println(stepName + " in progress.")
	fmt.Println()

	return &Step{
		stepName: stepName,
		step:     currentStep,
		store:    store,
		streams:  streams,
		verbose:  verbose,
		timer:    stopwatch.Start(),
	}, nil
}

func (s *Step) Err() error {
	return s.err
}

func (s *Step) RunHubSubstep(f func(streams step.OutStreams) error) {
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

func (s *Step) RunInternalSubstep(f func() error) {
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

func (s *Step) RunCLISubstep(substep idl.Substep, f func(streams step.OutStreams) error) {
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

func (s *Step) DisableStore() {
	s.store = nil
}

func (s *Step) Complete(completedText string) error {
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
		fmt.Println() // Separate the step status from the error text

		// allow substpes to override the default next actions
		var nextActions cli.NextActions
		if errors.As(s.Err(), &nextActions) {
			return nextActions
		}

		msg := fmt.Sprintf(`Please address the above issue and run "gpupgrade %s" again.`+additionalNextActions[s.step], strings.ToLower(s.stepName))
		return cli.NewNextActions(s.Err(), msg)
	}

	fmt.Println(completedText)
	return nil
}

func (s *Step) printStatus(substep idl.Substep, status idl.Status) {
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

func Prompt(reader *bufio.Reader) (bool, error) {
	for {
		fmt.Print("Continue with gpupgrade initialize?  Yy|Nn: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		input = strings.ToLower(strings.TrimSpace(input))
		switch input {
		case "y":
			fmt.Println()
			fmt.Print("Proceeding with upgrade")
			fmt.Println()
			return true, nil
		case "n":
			fmt.Println()
			fmt.Print("Canceling upgrade")
			return false, nil
		}
	}
}
