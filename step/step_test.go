package step_test

import (
	"errors"
	"io"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
)

func TestStepRun(t *testing.T) {
	t.Run("marks a successful substep run as complete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_CONFIG,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_CONFIG,
				Status: idl.Status_COMPLETE,
			}}})

		s := step.New("Initialize", server, &TestStore{}, DevNull)

		var called bool
		s.Run(idl.Substep_CONFIG, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("marks a failed substep run as failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_CONFIG,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_CONFIG,
				Status: idl.Status_FAILED,
			}}})

		s := step.New("Initialize", server, &TestStore{}, DevNull)

		var called bool
		s.Run(idl.Substep_CONFIG, func(streams step.OutStreams) error {
			called = true
			return errors.New("oops")
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("returns an error when MarkInProgress fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)

		failingStore := &TestStore{WriteErr: errors.New("oops")}
		s := step.New("Initialize", server, failingStore, DevNull)

		var called bool
		s.Run(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !xerrors.Is(s.Err(), failingStore.WriteErr) {
			t.Errorf("returned error %#v want %#v", s.Err(), failingStore.WriteErr)
		}

		if called {
			t.Error("expected substep to not be called")
		}
	})

	t.Run("skips completed substeps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{&idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_COMPLETE,
			}}})

		store := &TestStore{Status: idl.Status_COMPLETE}
		s := step.New("Initialize", server, store, DevNull)

		var called bool
		s.Run(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to be skipped")
		}
	})

	t.Run("on failure skips subsequent substeps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().Send(gomock.Any()).AnyTimes()

		s := step.New("Initialize", server, &TestStore{}, DevNull)

		expected := errors.New("oops")
		s.Run(idl.Substep_CONFIG, func(streams step.OutStreams) error {
			return expected
		})

		var called bool
		s.Run(idl.Substep_START_AGENTS, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to be skipped")
		}

		if !xerrors.Is(s.Err(), expected) {
			t.Errorf("got error %#v, want %#v", s.Err(), expected)
		}
	})

	t.Run("for a substep that was running mark it as failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().Send(gomock.Any()).AnyTimes()

		store := &TestStore{Status: idl.Status_RUNNING}
		s := step.New("Initialize", server, store, DevNull)

		var called bool
		s.Run(idl.Substep_CONFIG, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to not be called")
		}

		if s.Err() == nil {
			t.Error("got nil want err")
		}
	})
}

func TestStepFinish(t *testing.T) {
	t.Run("closes the output streams", func(t *testing.T) {
		streams := &devNull{}
		s := step.New("Initialize", nil, nil, streams)

		err := s.Finish()
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}

		if !streams.Closed {
			t.Errorf("stream was not closed")
		}
	})

	t.Run("returns an error when failing to close the output streams", func(t *testing.T) {
		expected := errors.New("oops")
		streams := &devNull{CloseErr: expected}
		s := step.New("Initialize", nil, nil, streams)

		err := s.Finish()
		if !xerrors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

type TestStore struct {
	Status   idl.Status
	WriteErr error
}

func (t *TestStore) Read(substep idl.Substep) (idl.Status, error) {
	return t.Status, nil
}

func (t *TestStore) Write(substep idl.Substep, status idl.Status) (err error) {
	return t.WriteErr
}

// DevNull implements step.OutStreamsCloser as a no-op. It also tracks calls to
// Close().
var DevNull = &devNull{}

type devNull struct {
	Closed   bool
	CloseErr error
}

func (devNull) Stdout() io.Writer {
	return ioutil.Discard
}

func (devNull) Stderr() io.Writer {
	return ioutil.Discard
}

func (d *devNull) Close() error {
	d.Closed = true
	return d.CloseErr
}
