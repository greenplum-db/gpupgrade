// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

const StepsFileName = "steps.json"

const nextActionRunRevertText = "If you would like to return the cluster to its original state, please run \"gpupgrade revert\".\n"

var additionalNextActions = map[idl.Step]string{
	idl.Step_initialize: nextActionRunRevertText,
	idl.Step_execute:    nextActionRunRevertText,
	idl.Step_finalize:   "",
	idl.Step_revert:     "",
}

type Step struct {
	stepName    string
	step        idl.Step
	stepStore   *StepStore
	streams     *step.BufferedStreams
	verbose     bool
	timer       *stopwatch.Stopwatch
	lastSubstep idl.Substep
	err         error
}

func NewStep(currentStep idl.Step, streams *step.BufferedStreams, verbose bool, interactive bool, confirmationText string) (*Step, error) {
	stepStore, err := NewStepStore()
	if err != nil {
		context := fmt.Sprintf("Note: If commands were issued in order, ensure gpupgrade can write to %s", utils.GetStateDir())
		wrappedErr := xerrors.Errorf("%v\n\n%v", StepErr, context)
		return &Step{}, utils.NewNextActionErr(wrappedErr, RunInitialize)
	}

	err = stepStore.ValidateStep(currentStep)
	if err != nil {
		return nil, err
	}

	if !interactive {
		fmt.Println(confirmationText)

		err := Prompt(bufio.NewReader(os.Stdin), currentStep)
		if err != nil {
			return &Step{}, err
		}
	}

	err = stepStore.Write(currentStep, idl.Status_running)
	if err != nil {
		return &Step{}, err
	}

	stepName := cases.Title(language.English).String(currentStep.String())

	fmt.Println()
	fmt.Println(stepName + " in progress.")
	fmt.Println()

	return &Step{
		stepName:  stepName,
		step:      currentStep,
		stepStore: stepStore,
		streams:   streams,
		verbose:   verbose,
		timer:     stopwatch.Start(),
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

	s.printStatus(substep, idl.Status_running)

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
		status := idl.Status_failed

		if errors.Is(err, step.Skip) {
			status = idl.Status_skipped
			err = nil
		}

		s.printStatus(substep, status)
		return
	}

	s.printStatus(substep, idl.Status_complete)
}

func (s *Step) DisableStore() {
	s.stepStore = nil
}

func (s *Step) Complete(completedText string) error {
	logDuration(s.stepName, s.verbose, s.timer.Stop())

	status := idl.Status_complete
	if s.Err() != nil {
		status = idl.Status_failed
	}

	if s.stepStore != nil {
		if wErr := s.stepStore.Write(s.step, status); wErr != nil {
			s.err = errorlist.Append(s.err, wErr)
		}
	}

	if s.Err() != nil {
		fmt.Println() // Separate the step status from the error text

		genericNextAction := fmt.Sprintf("Please address the above issue and run \"gpupgrade %s\" again.\n"+additionalNextActions[s.step], strings.ToLower(s.stepName))

		var nextActionErr utils.NextActionErr
		if errors.As(s.Err(), &nextActionErr) {
			return utils.NewNextActionErr(s.Err(), nextActionErr.NextAction+"\n\n"+genericNextAction)
		}

		return utils.NewNextActionErr(s.Err(), genericNextAction)
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
	if status != idl.Status_running {
		fmt.Println()
	}

	s.lastSubstep = substep
}

func logDuration(operation string, verbose bool, timer *stopwatch.Stopwatch) {
	msg := operation + " took " + timer.String()
	if verbose {
		fmt.Println(msg)
		fmt.Println()
		fmt.Println("-----------------------------------------------------------------------------")
		fmt.Println()
	}
	log.Print(msg)
}

func Prompt(reader *bufio.Reader, currentStep idl.Step) error {
	for {
		fmt.Printf("Continue with gpupgrade %s?  Yy|Nn: ", currentStep)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		input = strings.ToLower(strings.TrimSpace(input))
		switch input {
		case "y":
			fmt.Println()
			fmt.Print("Proceeding with upgrade")
			fmt.Println()
			return nil
		case "n":
			fmt.Println()
			fmt.Print("Canceling...")
			return step.UserCanceled
		}
	}
}
