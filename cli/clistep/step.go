// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package clistep

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/substeps"
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
	streams      step.OutStreams
	verbose      bool
	stepTimer    *stopwatch.Stopwatch
	lastSubstep  idl.Substep
	err          error
}

func NewStep(currentStep idl.Step, stepName string, stepStore StepStore, substepStore step.SubstepStore, streams step.OutStreams, verbose bool) (*Step, error) {
	return &Step{
		stepName:     stepName,
		step:         currentStep,
		stepStore:    stepStore,
		substepStore: substepStore,
		streams:      streams,
		verbose:      verbose,
		stepTimer:    stopwatch.Start(),
	}, nil
}

func Begin(currentStep idl.Step, verbose bool, nonInteractive bool, confirmationText string) (*Step, error) {
	// NOTE: only use streams within the substeps since they do not write to
	// stdout/stderr when verbose is false. Thus, for general output write to
	// stdout as usual such that it appears when verbose is not set.
	streams := step.NewLogStdStreams(verbose)

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

	log.Print(confirmationText)

	if !nonInteractive {
		fmt.Print(confirmationText)

		prompt := fmt.Sprintf("Continue with gpupgrade %s?  Yy|Nn: ", currentStep)
		err := Prompt(utils.StdinReader, prompt)
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

	text := fmt.Sprintf("\n%s in progress.\n\n", stepName)
	fmt.Print(text)
	log.Print(text)

	return NewStep(currentStep, stepName, stepStore, substepStore, streams, verbose)
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
		log.Printf("%s skipped. Run condition not met.", substeps.SubstepDescriptions[substep].HelpText)
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
		if s.err == nil {
			if _, pErr := fmt.Fprintf(s.streams.Stdout(), "\n\n%s\n\n", substeps.Divider); pErr != nil {
				err = errorlist.Append(err, pErr)
			}
		}

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

		return
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
		pErr := s.printDuration(substeps.SubstepDescriptions[substep].OutputText, substepTimer.Stop().String())
		if pErr != nil {
			err = errorlist.Append(err, pErr)
			return
		}
	}()

	if pErr := s.printStatus(substep, idl.Status_running); pErr != nil {
		err = errorlist.Append(err, pErr)
		return
	}

	err = f(s.streams)
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
	if pErr := s.printDuration(s.stepName, s.stepTimer.Stop().String()); pErr != nil {
		s.err = errorlist.Append(s.err, pErr)
	}

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
		if s.verbose {
			fmt.Println()
		}

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

	if s.verbose {
		fmt.Println()
	}

	text := fmt.Sprintf("\n%s completed successfully.\n", s.stepName)
	fmt.Print(text)
	log.Print(text)

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

	text := substeps.SubstepDescriptions[substep].OutputText
	fmt.Print(commanders.Format(text, status))
	log.Print(commanders.Format(text, status))

	// Reset the cursor if the final status has been written. This prevents the
	// status from a hub step from being on the same line as a CLI step.
	if status != idl.Status_running || s.verbose {
		fmt.Println()
	}

	s.lastSubstep = substep
	return nil
}

func (s *Step) printDuration(operation string, duration string) error {
	_, err := fmt.Fprintf(s.streams.Stdout(), "%-67s[%s]", operation, duration)
	return err
}

func Prompt(reader *bufio.Reader, prompt string) error {
	fmt.Println()
	for {
		fmt.Print(prompt)
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
