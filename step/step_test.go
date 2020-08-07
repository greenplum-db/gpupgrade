// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestStepRun(t *testing.T) {
	t.Run("marks a successful substep as complete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_COMPLETE,
			}}})

		s := step.New(idl.Step_INITIALIZE, server, &TestStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("run correctly sets the substep status", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_COMPLETE,
			}}})

		store := &TestStore{}
		s := step.New(idl.Step_INITIALIZE, server, store, &testutils.DevNullWithClose{})

		var status idl.Status
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			// save off status to verify that it is running
			status = store.Status
			return nil
		})

		expected := idl.Status_RUNNING
		if status != expected {
			t.Errorf("got %q want %q", status, expected)
		}

		expected = idl.Status_COMPLETE
		if store.Status != expected {
			t.Errorf("got %q want %q", store.Status, expected)
		}
	})

	t.Run("AlwaysRun re-runs a completed substep", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_COMPLETE,
			}}})

		store := &TestStore{Status: idl.Status_COMPLETE}
		s := step.New(idl.Step_INITIALIZE, server, store, &testutils.DevNullWithClose{})

		var called bool
		s.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("marks a failed substep as failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_RUNNING,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
				Status: idl.Status_FAILED,
			}}})

		s := step.New(idl.Step_INITIALIZE, server, &TestStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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
		s := step.New(idl.Step_INITIALIZE, server, failingStore, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !errors.Is(s.Err(), failingStore.WriteErr) {
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
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_COMPLETE,
			}}})

		store := &TestStore{Status: idl.Status_COMPLETE}
		s := step.New(idl.Step_INITIALIZE, server, store, &testutils.DevNullWithClose{})

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

		s := step.New(idl.Step_INITIALIZE, server, &TestStore{}, &testutils.DevNullWithClose{})

		expected := errors.New("oops")
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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

		if !errors.Is(s.Err(), expected) {
			t.Errorf("got error %#v, want %#v", s.Err(), expected)
		}
	})

	t.Run("for a substep that was running mark it as failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().Send(gomock.Any()).AnyTimes()

		store := &TestStore{Status: idl.Status_RUNNING}
		s := step.New(idl.Step_INITIALIZE, server, store, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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

func TestHasRun(t *testing.T) {
	cases := []struct {
		description string
		status      idl.Status
	}{
		{
			description: "returns true when substep is running",
			status:      idl.Status_RUNNING,
		},
		{
			description: "returns true when substep has completed",
			status:      idl.Status_COMPLETE,
		},
		{
			description: "returns true when substep has errored",
			status:      idl.Status_FAILED,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			dir := testutils.GetTempDir(t, "")
			defer testutils.MustRemoveAll(t, dir)

			resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
			defer resetEnv()

			path := filepath.Join(dir, "status.json")
			testutils.MustWriteToFile(t, path, "{}")
			store := step.NewFileStore(path)
			err := store.Write(idl.Step_INITIALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, c.status)
			if err != nil {
				t.Errorf("store.Write returned error %+v", err)
			}

			hasRun, err := step.HasRun(idl.Step_INITIALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)
			if err != nil {
				t.Errorf("HasRun returned error %+v", err)
			}

			if !hasRun {
				t.Errorf("expected substep to have been run")
			}
		})
	}

	t.Run("returns an error when getting the status file fails", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", "does/not/exist")
		defer resetEnv()

		hasRun, err := step.HasRun(idl.Step_INITIALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}

		if hasRun {
			t.Errorf("expected substep to not have been run")
		}
	})

	t.Run("returns an error when reading from the store fails", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, "status.json")
		testutils.MustWriteToFile(t, path, `{"}"`) // write a malformed JSON status file

		hasRun, err := step.HasRun(idl.Step_INITIALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}

		if hasRun {
			t.Errorf("expected substep to not have been run")
		}
	})

	t.Run("returns false with no error when a step has not yet been run", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, "status.json")
		testutils.MustWriteToFile(t, path, "{}")

		hasRan, err := step.HasRun(idl.Step_FINALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)
		if err != nil {
			t.Errorf("HasRun returned error %+v", err)
		}

		if hasRan {
			t.Errorf("expected substep to not have been run")
		}
	})
}

func TestStepFinish(t *testing.T) {
	t.Run("closes the output streams", func(t *testing.T) {
		streams := &testutils.DevNullWithClose{}
		s := step.New(idl.Step_INITIALIZE, nil, nil, streams)

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
		streams := &testutils.DevNullWithClose{CloseErr: expected}
		s := step.New(idl.Step_INITIALIZE, nil, nil, streams)

		err := s.Finish()
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestStatusFile(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	path := filepath.Join(stateDir, "status.json")

	t.Run("creates status file if it does not exist", func(t *testing.T) {
		_, err := os.Open(path)
		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v want ErrNotExist", err)
		}

		statusFile, err := step.GetStatusFile(stateDir)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		contents := testutils.MustReadFile(t, statusFile)
		if contents != "{}" {
			t.Errorf("read %q want {}", contents)
		}
	})

	t.Run("does not create status file if it already exists", func(t *testing.T) {
		expected := "1234"
		testutils.MustWriteToFile(t, path, expected)

		statusFile, err := step.GetStatusFile(stateDir)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}

		contents := testutils.MustReadFile(t, statusFile)
		if contents != expected {
			t.Errorf("read %q want %q", contents, expected)
		}
	})
}

type TestStore struct {
	Status   idl.Status
	WriteErr error
}

func (t *TestStore) Read(_ idl.Step, substep idl.Substep) (idl.Status, error) {
	return t.Status, nil
}

func (t *TestStore) Write(_ idl.Step, substep idl.Substep, status idl.Status) (err error) {
	t.Status = status
	return t.WriteErr
}
