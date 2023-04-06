// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
	"github.com/greenplum-db/gpupgrade/utils/logger"
	"github.com/greenplum-db/gpupgrade/utils/stopwatch"
)

const SubstepsFileName = "substeps.json"

type Step struct {
	name         idl.Step
	sender       idl.MessageSender // sends substep status messages
	substepStore SubstepStore      // persistent substep status storage
	streams      OutStreamsCloser  // writes substep stdout/err
	err          error
}

func New(name idl.Step, sender idl.MessageSender, substepStore SubstepStore, streams OutStreamsCloser) *Step {
	return &Step{
		name:         name,
		sender:       sender,
		substepStore: substepStore,
		streams:      streams,
	}
}

func Begin(step idl.Step, sender idl.MessageSender, agentConns func() ([]*idl.Connection, error)) (*Step, error) {
	// FIXME: Having s.agentConns() in the step framework is a heavy indication of
	//  tech debt that needs to be addressed. However, for the time being ensure
	//  agentConns are properly populated at the start of each step, otherwise
	//  the agentConns member variable can be nil.
	agentsStarted, err := HasCompleted(idl.Step_initialize, idl.Substep_start_agents)
	if err != nil {
		return nil, err
	}

	if agentsStarted {
		_, err := agentConns()
		if err != nil {
			return nil, xerrors.Errorf("ensuring agent connections are ready: %w", err)
		}
	}

	logFile, err := logger.OpenFile("hub")
	if err != nil {
		return nil, xerrors.Errorf(`getting log file for step "%s": %w`, step, err)
	}

	_, err = fmt.Fprintf(logFile, "\n%s in progress.\n", cases.Title(language.English).String(step.String()))
	if err != nil {
		return nil, xerrors.Errorf(`logging step "%s": %w`, step, err)
	}

	substepStore, err := NewSubstepFileStore()
	if err != nil {
		return nil, err
	}

	streams := newMultiplexedStream(sender, logFile)

	return New(step, sender, substepStore, streams), nil
}

func HasStarted(step idl.Step) (bool, error) {
	substepStore, err := NewSubstepFileStore()
	if err != nil {
		return false, err
	}

	substepsMap, err := substepStore.ReadStep(step)
	if err != nil {
		return false, err
	}

	if substepsMap != nil {
		return true, nil
	}

	return false, nil
}

func HasRun(step idl.Step, substep idl.Substep) (bool, error) {
	return hasStatus(step, substep, func(status idl.Status) bool {
		return status != idl.Status_unknown_status
	})
}

func HasCompleted(step idl.Step, substep idl.Substep) (bool, error) {
	return hasStatus(step, substep, func(status idl.Status) bool {
		return status == idl.Status_complete
	})
}

func hasStatus(step idl.Step, substep idl.Substep, check func(status idl.Status) bool) (bool, error) {
	substepStore, err := NewSubstepFileStore()
	if err != nil {
		return false, err
	}

	status, err := substepStore.Read(step, substep)
	if err != nil {
		return false, err
	}

	return check(status), nil
}

func (s *Step) Streams() OutStreams {
	return s.streams
}

func (s *Step) Finish() error {
	if err := s.streams.Close(); err != nil {
		return xerrors.Errorf(`step "%s": %w`, s.name, err)
	}

	return nil
}

func (s *Step) Err() error {
	if s.err == nil {
		return nil
	}

	text := ""
	var nextActionErr utils.NextActionErr
	if errors.As(s.err, &nextActionErr) {
		text += nextActionErr.NextAction
	}

	var errs errorlist.Errors
	if errors.As(s.err, &errs) {
		var nextActions []string
		for _, err := range errs {
			if errors.As(err, &nextActionErr) {
				nextActions = append(nextActions, nextActionErr.NextAction)
			}
		}

		text = strings.Join(nextActions, "\n")
	}

	if text == "" {
		return s.err
	}

	statusErr := status.New(codes.Internal, s.err.Error())
	statusErr, err := statusErr.WithDetails(&idl.NextActions{NextActions: text})
	if err != nil {
		return s.err
	}

	return statusErr.Err()
}

func (s *Step) RunInternalSubstep(f func() error) {
	if s.err != nil {
		return
	}

	err := f()
	if err != nil {
		s.err = err
	}
}

func (s *Step) AlwaysRun(substep idl.Substep, f func(OutStreams) error) {
	s.run(substep, f, true)
}

func (s *Step) RunConditionally(substep idl.Substep, shouldRun bool, f func(OutStreams) error) {
	if !shouldRun {
		log.Printf("skipping %s", substep)
		return
	}

	s.run(substep, f, false)
}

func (s *Step) Run(substep idl.Substep, f func(OutStreams) error) {
	s.run(substep, f, false)
}

func (s *Step) run(substep idl.Substep, f func(OutStreams) error, alwaysRun bool) {
	var err error
	defer func() {
		if err != nil {
			s.err = xerrors.Errorf(`substep "%s": %w`, substep, err)
		}
	}()

	if s.err != nil {
		return
	}

	status, err := s.substepStore.Read(s.name, substep)
	if err != nil {
		return
	}

	if status == idl.Status_running {
		// TODO: Finalize error wording and recommended action
		err = fmt.Errorf("Found previous substep %s was running. Manual intervention needed to cleanup. Please contact support.", substep)
		s.sendStatus(substep, idl.Status_failed)
		return
	}

	// Only re-run substeps that are failed or pending. Do not skip substeps that must always be run.
	if status == idl.Status_complete && !alwaysRun {
		// Only send the status back to the UI; don't re-persist to the store
		s.sendStatus(substep, idl.Status_skipped)
		return
	}

	timer := stopwatch.Start()
	defer func() {
		if pErr := s.printDuration(substep, timer.Stop()); pErr != nil {
			err = errorlist.Append(err, pErr)
		}
	}()

	_, err = fmt.Fprintf(s.streams.Stdout(), "\nStarting %s...\n\n", substep)
	if err != nil {
		return
	}

	err = s.write(substep, idl.Status_running)
	if err != nil {
		return
	}

	err = f(s.streams)

	switch {
	case errors.Is(err, Skip):
		// The substep has requested a manual skip; this isn't really an error.
		err = s.write(substep, idl.Status_skipped)
		return

	case err != nil:
		if werr := s.write(substep, idl.Status_failed); werr != nil {
			err = errorlist.Append(err, werr)
		}
		return
	}

	err = s.write(substep, idl.Status_complete)
}

func (s *Step) write(substep idl.Substep, status idl.Status) error {
	storeStatus := status
	if status == idl.Status_skipped {
		// Special case: we want to mark an explicitly-skipped substep COMPLETE
		// on disk.
		storeStatus = idl.Status_complete
	}

	err := s.substepStore.Write(s.name, substep, storeStatus)
	if err != nil {
		return err
	}

	s.sendStatus(substep, status)
	return nil
}

func (s *Step) sendStatus(substep idl.Substep, status idl.Status) {
	// A stream is not guaranteed to remain connected during execution, so
	// errors are explicitly ignored.
	_ = s.sender.Send(&idl.Message{
		Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
			Step:   substep,
			Status: status,
		}},
	})
}

func (s *Step) printDuration(substep idl.Substep, timer *stopwatch.Stopwatch) error {
	divider := "-----------------------------------------------------------------------------"
	_, err := fmt.Fprintf(s.streams.Stdout(), "\n%s took %s\n\n%s\n", substep, timer.String(), divider)
	return err
}

// Skip can be returned from a Run or AlwaysRun callback to immediately mark the
// substep complete on disk and report "skipped" to the UI.
var Skip = skipErr{}

type skipErr struct{}

func (s skipErr) Error() string { return "skipped" }

// Quit indicates that the user has canceled and does not want to proceed.
var Quit = userQuitErr{}

type userQuitErr struct{}

func (s userQuitErr) Error() string { return "user quit" }
