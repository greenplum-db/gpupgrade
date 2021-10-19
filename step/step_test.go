// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/greenplum-db/gpupgrade/testutils/testlog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestStepRun(t *testing.T) {
	_, _, log := testlog.SetupLogger()

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

		s := step.New(idl.Step_INITIALIZE, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("reports an explicitly skipped substep and marks the status complete on disk", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)

		gomock.InOrder(
			server.EXPECT().
				Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
					Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
					Status: idl.Status_RUNNING,
				}}}),
			server.EXPECT().
				Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
					Step:   idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG,
					Status: idl.Status_SKIPPED,
				}}}),
		)

		substepStore := &TestSubstepStore{}
		s := step.New(idl.Step_INITIALIZE, server, substepStore, &testutils.DevNullWithClose{})

		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			return step.Skip
		})

		if substepStore.Status != idl.Status_COMPLETE {
			t.Errorf("substep status was %s, want %s", substepStore.Status, idl.Status_COMPLETE)
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

		substepStore := &TestSubstepStore{}
		s := step.New(idl.Step_INITIALIZE, server, substepStore, &testutils.DevNullWithClose{})

		var status idl.Status
		s.Run(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			// save off status to verify that it is running
			status = substepStore.Status
			return nil
		})

		expected := idl.Status_RUNNING
		if status != expected {
			t.Errorf("got %q want %q", status, expected)
		}

		expected = idl.Status_COMPLETE
		if substepStore.Status != expected {
			t.Errorf("got %q want %q", substepStore.Status, expected)
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

		substepStore := &TestSubstepStore{Status: idl.Status_COMPLETE}
		s := step.New(idl.Step_INITIALIZE, server, substepStore, &testutils.DevNullWithClose{})

		var called bool
		s.AlwaysRun(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("RunConditionally logs and does not run substep when shouldRun is false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_RUNNING,
			}}}).Times(0)

		s := step.New(idl.Step_INITIALIZE, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.RunConditionally(idl.Substep_CHECK_UPGRADE, false, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to not be called")
		}

		contents := string(log.Bytes())
		expected := "skipping " + idl.Substep_CHECK_UPGRADE.String()
		if !strings.Contains(contents, expected) {
			t.Errorf("expected %q in log file: %q", expected, contents)
		}
	})

	t.Run("RunConditionally runs substep when shouldRun is true", func(t *testing.T) {
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

		s := step.New(idl.Step_INITIALIZE, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.RunConditionally(idl.Substep_CHECK_UPGRADE, true, func(streams step.OutStreams) error {
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

		s := step.New(idl.Step_INITIALIZE, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

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

		failingSubstepStore := &TestSubstepStore{WriteErr: errors.New("oops")}
		s := step.New(idl.Step_INITIALIZE, server, failingSubstepStore, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_CHECK_UPGRADE, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !errors.Is(s.Err(), failingSubstepStore.WriteErr) {
			t.Errorf("returned error %#v want %#v", s.Err(), failingSubstepStore.WriteErr)
		}

		if called {
			t.Error("expected substep to not be called")
		}
	})

	t.Run("skips completed substeps and sends a skipped status to the client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_CHECK_UPGRADE,
				Status: idl.Status_SKIPPED,
			}}})

		substepStore := &TestSubstepStore{Status: idl.Status_COMPLETE}
		s := step.New(idl.Step_INITIALIZE, server, substepStore, &testutils.DevNullWithClose{})

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

		s := step.New(idl.Step_INITIALIZE, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

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

		substepStore := &TestSubstepStore{Status: idl.Status_RUNNING}
		s := step.New(idl.Step_INITIALIZE, server, substepStore, &testutils.DevNullWithClose{})

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

func TestHasStarted(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	t.Run("returns an error when getting the status file fails", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", "does/not/exist")
		defer resetEnv()

		hasStarted, err := step.HasStarted(idl.Step_INITIALIZE)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}

		if hasStarted {
			t.Errorf("expected step to not have been run")
		}
	})

	t.Run("returns an error when reading from the substep store fails", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, step.SubstepsFileName)
		testutils.MustWriteToFile(t, path, `{"}"`) // write a malformed JSON status file

		hasStarted, err := step.HasStarted(idl.Step_INITIALIZE)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}

		if hasStarted {
			t.Errorf("expected step to not have been run")
		}
	})

	t.Run("returns false with no error when a step has not yet been started", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, step.SubstepsFileName)
		testutils.MustWriteToFile(t, path, "{}")

		hasStarted, err := step.HasStarted(idl.Step_FINALIZE)
		if err != nil {
			t.Errorf("HasStarted returned error %+v", err)
		}

		if hasStarted {
			t.Errorf("expected step to not have been run")
		}
	})

	t.Run("returns true with no error when a step has been started", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, step.SubstepsFileName)
		jsonContent := fmt.Sprintf("{\"%s\":{\"%s\":\"%s\"}}",
			idl.Step_INITIALIZE, idl.Substep_BACKUP_TARGET_MASTER, idl.Status_COMPLETE)
		testutils.MustWriteToFile(t, path, jsonContent)

		hasStarted, err := step.HasStarted(idl.Step_INITIALIZE)
		if err != nil {
			t.Errorf("HasStarted returned error %+v", err)
		}

		if !hasStarted {
			t.Errorf("expected substep to not have been run")
		}
	})
}

func TestHasRun(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

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
			store, err := step.NewSubstepFileStore()
			if err != nil {
				t.Fatalf("step.NewSubstepStore returned error %+v", err)
			}

			err = store.Write(idl.Step_INITIALIZE, idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, c.status)
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

	t.Run("returns an error when reading from the substep store fails", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, step.SubstepsFileName)
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

		path := filepath.Join(dir, step.SubstepsFileName)
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

func TestHasCompleted(t *testing.T) {
	stateDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	cases := []struct {
		description string
		status      idl.Status
		expected    bool
	}{
		{
			description: "returns false when substep is running",
			status:      idl.Status_RUNNING,
			expected:    false,
		},
		{
			description: "returns true when substep has completed",
			status:      idl.Status_COMPLETE,
			expected:    true,
		},
		{
			description: "returns false when substep has errored",
			status:      idl.Status_FAILED,
			expected:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			store, err := step.NewSubstepFileStore()
			if err != nil {
				t.Fatalf("step.NewSubstepStore returned error %+v", err)
			}

			err = store.Write(idl.Step_INITIALIZE, idl.Substep_START_AGENTS, c.status)
			if err != nil {
				t.Errorf("store.Write returned error %+v", err)
			}

			hasRun, err := step.HasCompleted(idl.Step_INITIALIZE, idl.Substep_START_AGENTS)
			if err != nil {
				t.Errorf("HasRun returned error %+v", err)
			}

			if hasRun != c.expected {
				t.Errorf("substep status %t want %t", hasRun, c.expected)
			}
		})
	}

	t.Run("returns an error when getting the status file fails", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", "does/not/exist")
		defer resetEnv()

		hasRun, err := step.HasCompleted(idl.Step_INITIALIZE, idl.Substep_START_AGENTS)
		var expected *os.PathError
		if !errors.As(err, &expected) {
			t.Errorf("returned error %#v want %#v", err, expected)
		}

		if hasRun {
			t.Errorf("expected substep to not have been run")
		}
	})

	t.Run("returns an error when reading from the substep store fails", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", dir)
		defer resetEnv()

		path := filepath.Join(dir, step.SubstepsFileName)
		testutils.MustWriteToFile(t, path, `{"}"`) // write a malformed JSON status file

		hasRun, err := step.HasCompleted(idl.Step_INITIALIZE, idl.Substep_START_AGENTS)
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

		path := filepath.Join(dir, step.SubstepsFileName)
		testutils.MustWriteToFile(t, path, "{}")

		hasRan, err := step.HasCompleted(idl.Step_FINALIZE, idl.Substep_START_AGENTS)
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

type TestSubstepStore struct {
	Status   idl.Status
	WriteErr error
}

func (t *TestSubstepStore) Read(_ idl.Step, substep idl.Substep) (idl.Status, error) {
	return t.Status, nil
}

func (t *TestSubstepStore) Write(_ idl.Step, substep idl.Substep, status idl.Status) (err error) {
	t.Status = status
	return t.WriteErr
}
