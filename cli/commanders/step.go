// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
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
	stepName     string
	step         idl.Step
	stepStore    StepStore
	substepStore step.SubstepStore
	streams      *step.BufferedStreams
	verbose      bool
	timer        *stopwatch.Stopwatch
	lastSubstep  idl.Substep
	err          error
}

func NewStep(currentStep idl.Step, stepName string, stepStore StepStore, substepStore step.SubstepStore, streams *step.BufferedStreams, verbose bool) (*Step, error) {
	return &Step{
		stepName:     stepName,
		step:         currentStep,
		stepStore:    stepStore,
		substepStore: substepStore,
		streams:      streams,
		verbose:      verbose,
		timer:        stopwatch.Start(),
	}, nil
}

func Begin(currentStep idl.Step, verbose bool, nonInteractive bool, confirmationText string) (*Step, error) {
	stepStore, err := NewStepFileStore()
	if err != nil {
		context := fmt.Sprintf("Note: If commands were issued in order, ensure gpupgrade can write to %s", utils.GetStateDir())
		wrappedErr := xerrors.Errorf("%v\n\n%v", StepErr, context)
		return &Step{}, utils.NewNextActionErr(wrappedErr, RunInitialize)
	}

	err = stepStore.ValidateStep(currentStep)
	if err != nil {
		return nil, err
	}

	if !nonInteractive {
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

	substepStore, err := step.NewSubstepFileStore()
	if err != nil {
		return nil, err
	}

	stepName := cases.Title(language.English).String(currentStep.String())

	fmt.Println()
	fmt.Println(stepName + " in progress.")
	fmt.Println()

	return NewStep(currentStep, stepName, stepStore, substepStore, &step.BufferedStreams{}, verbose)
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

func (s *Step) AlwaysRun(substep idl.Substep, f func(streams step.OutStreams) error) {
	s.run(substep, f, true)
}

func (s *Step) RunConditionally(substep idl.Substep, shouldRun bool, f func(streams step.OutStreams) error) {
	if !shouldRun {
		log.Printf("skipping %s", substep)
		return
	}

	s.run(substep, f, false)
}

func (s *Step) Run(substep idl.Substep, f func(streams step.OutStreams) error) {
	s.run(substep, f, false)
}

func (s *Step) run(substep idl.Substep, f func(streams step.OutStreams) error, alwaysRun bool) {
	var err error
	defer func() {
		if err != nil {
			s.err = xerrors.Errorf("substep %q: %w", substep, err)
		}
	}()

	if s.err != nil {
		return
	}

	status, rErr := s.substepStore.Read(s.step, substep)
	if rErr != nil {
		err = errorlist.Append(err, rErr)
		return
	}

	if status == idl.Status_running {
		err = fmt.Errorf("Found previous substep %s was running. Manual intervention needed to cleanup. Please contact support.", substep)
		if pErr := s.printStatus(substep, idl.Status_failed); pErr != nil {
			err = errorlist.Append(err, pErr)
			return
		}
	}

	// Only re-run substeps that are failed or pending. Do not skip substeps that must always be run.
	if status == idl.Status_complete && !alwaysRun {
		if pErr := s.printStatus(substep, idl.Status_skipped); pErr != nil {
			err = errorlist.Append(err, pErr)
			return
		}

		return
	}

	substepTimer := stopwatch.Start()
	defer func() {
		logDuration(substep.String(), s.verbose, substepTimer.Stop())
	}()

	if pErr := s.printStatus(substep, idl.Status_running); pErr != nil {
		err = errorlist.Append(err, pErr)
		return
	}

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

		if errors.Is(err, step.Quit) {
			status = idl.Status_quit
		}

		if pErr := s.printStatus(substep, status); pErr != nil {
			err = errorlist.Append(err, pErr)
			return
		}

		return
	}

	if pErr := s.printStatus(substep, idl.Status_complete); pErr != nil {
		err = errorlist.Append(err, pErr)
		return
	}
}

func (s *Step) DisableStore() {
	s.stepStore = nil
	s.substepStore = nil
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

		if errors.Is(s.Err(), step.Quit) {
			return s.Err()
		}

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

func (s *Step) printStatus(substep idl.Substep, status idl.Status) error {
	if substep == s.lastSubstep {
		// For the same substep reset the cursor to overwrite the current status.
		fmt.Print("\r")
	}

	storeStatus := status
	if status == idl.Status_skipped {
		// Special case: we want to mark an explicitly-skipped substep complete on disk.
		storeStatus = idl.Status_complete
	}

	if s.substepStore != nil {
		err := s.substepStore.Write(s.step, substep, storeStatus)
		if err != nil {
			return err
		}
	}

	text := SubstepDescriptions[substep]
	fmt.Print(Format(text.OutputText, status))

	// Reset the cursor if the final status has been written. This prevents the
	// status from a hub step from being on the same line as a CLI step.
	if status != idl.Status_running {
		fmt.Println()
	}

	s.lastSubstep = substep
	return nil
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
			return step.Quit
		}
	}
}
