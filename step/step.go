package step

import (
	"fmt"
	"io"

	multierror "github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
)

type Step struct {
	name    string
	sender  idl.MessageSender // sends substep status messages
	store   Store             // persistent substep status storage
	streams OutStreamsCloser  // writes substep stdout/err
	err     error
}

type Store interface {
	Read(idl.Substep) (idl.Status, error)
	Write(idl.Substep, idl.Status) error
}

type OutStreams interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

type OutStreamsCloser interface {
	OutStreams
	Close() error
}

func New(name string, sender idl.MessageSender, store Store, streams OutStreamsCloser) *Step {
	return &Step{
		name:    name,
		sender:  sender,
		store:   store,
		streams: streams,
	}
}

func (s *Step) Finish() error {
	if err := s.streams.Close(); err != nil {
		return xerrors.Errorf(`step "%s": %w`, s.name, err)
	}

	return nil
}

func (s *Step) Err() error {
	return s.err
}

func (s *Step) AlwaysRun(substep idl.Substep, f func(OutStreams) error) {
	s.run(substep, f, true)
}

func (s *Step) Run(substep idl.Substep, f func(OutStreams) error) {
	s.run(substep, f, false)
}

func (s *Step) run(substep idl.Substep, f func(OutStreams) error, alwaysRun bool) {
	var err error
	defer func() {
		if err != nil {
			s.err = xerrors.Errorf(`substep "%s": %w`, s.name, err)
		}
	}()

	if s.err != nil {
		return
	}

	status, err := s.store.Read(substep)
	if err != nil {
		return
	}

	if status == idl.Status_RUNNING {
		// TODO: Finalize error wording and recommended action
		err = fmt.Errorf("Found previous substep %s was running. Manual intervention needed to cleanup. Please contact support.", substep)
		s.sendStatus(substep, idl.Status_FAILED)
		return
	}

	// Only re-run substeps that are failed or pending. Do not skip substeps that must always be run.
	if status == idl.Status_COMPLETE && !alwaysRun {
		// Only send the status back to the UI; don't re-persist to the store
		s.sendStatus(substep, idl.Status_COMPLETE)
		return
	}

	_, err = fmt.Fprintf(s.streams.Stdout(), "\nStarting %s...\n\n", substep)
	if err != nil {
		return
	}

	err = s.write(substep, idl.Status_RUNNING)
	if err != nil {
		return
	}

	err = f(s.streams)
	if err != nil {
		if werr := s.write(substep, idl.Status_FAILED); werr != nil {
			err = multierror.Append(err, werr).ErrorOrNil()
		}
		return
	}

	err = s.write(substep, idl.Status_COMPLETE)
}

func (s *Step) write(substep idl.Substep, status idl.Status) error {
	err := s.store.Write(substep, status)
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
		Contents: &idl.Message_Status{&idl.SubstepStatus{
			Step:   substep,
			Status: status,
		}},
	})
}
