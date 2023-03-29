// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/status"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func TestStepRun(t *testing.T) {
	logOutput := testlog.SetupTestLogger()

	t.Run("marks a successful substep as complete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_complete,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
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
					Step:   idl.Substep_saving_source_cluster_config,
					Status: idl.Status_running,
				}}}),
			server.EXPECT().
				Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
					Step:   idl.Substep_saving_source_cluster_config,
					Status: idl.Status_skipped,
				}}}),
		)

		substepStore := &TestSubstepStore{}
		s := step.New(idl.Step_initialize, server, substepStore, &testutils.DevNullWithClose{})

		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return step.Skip
		})

		if substepStore.Status != idl.Status_complete {
			t.Errorf("substep status was %s, want %s", substepStore.Status, idl.Status_complete)
		}
	})

	t.Run("run correctly sets the substep status", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_complete,
			}}})

		substepStore := &TestSubstepStore{}
		s := step.New(idl.Step_initialize, server, substepStore, &testutils.DevNullWithClose{})

		var status idl.Status
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			// save off status to verify that it is running
			status = substepStore.Status
			return nil
		})

		expected := idl.Status_running
		if status != expected {
			t.Errorf("got %q want %q", status, expected)
		}

		expected = idl.Status_complete
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
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_complete,
			}}})

		substepStore := &TestSubstepStore{Status: idl.Status_complete}
		s := step.New(idl.Step_initialize, server, substepStore, &testutils.DevNullWithClose{})

		var called bool
		s.AlwaysRun(idl.Substep_check_upgrade, func(streams step.OutStreams) error {
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
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_running,
			}}}).Times(0)

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.RunConditionally(idl.Substep_check_upgrade, false, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to not be called")
		}

		contents := string(logOutput.Bytes())
		expected := "skipping " + idl.Substep_check_upgrade.String()
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
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_complete,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.RunConditionally(idl.Substep_check_upgrade, true, func(streams step.OutStreams) error {
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
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_failed,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
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
		s := step.New(idl.Step_initialize, server, failingSubstepStore, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_check_upgrade, func(streams step.OutStreams) error {
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
				Step:   idl.Substep_check_upgrade,
				Status: idl.Status_skipped,
			}}})

		substepStore := &TestSubstepStore{Status: idl.Status_complete}
		s := step.New(idl.Step_initialize, server, substepStore, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_check_upgrade, func(streams step.OutStreams) error {
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

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		expected := errors.New("oops")
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return expected
		})

		var called bool
		s.Run(idl.Substep_start_agents, func(streams step.OutStreams) error {
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

		substepStore := &TestSubstepStore{Status: idl.Status_running}
		s := step.New(idl.Step_initialize, server, substepStore, &testutils.DevNullWithClose{})

		var called bool
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
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

	t.Run("when setting up for specific substeps to run it does not run substeps that does not match", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_check_active_connections_on_target_cluster,
				Status: idl.Status_running,
			}}}).Times(0)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_archive_log_directories,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_archive_log_directories,
				Status: idl.Status_complete,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_shutdown_target_cluster,
				Status: idl.Status_running,
			}}}).Times(0)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_delete_backupdir,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_delete_backupdir,
				Status: idl.Status_complete,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_delete_segment_statedirs,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_delete_segment_statedirs,
				Status: idl.Status_complete,
			}}})

		substepStore := &TestSubstepStore{Status: idl.Status_unknown_status}
		s := step.New(idl.Step_revert, server, substepStore, &testutils.DevNullWithClose{})

		s.OnlyRun(
			idl.Substep_archive_log_directories,
			idl.Substep_delete_backupdir,
			idl.Substep_delete_segment_statedirs)

		s.Run(idl.Substep_check_active_connections_on_target_cluster, func(streams step.OutStreams) error {
			t.Error("expected substep to not be called")
			return nil
		})

		var called int
		s.Run(idl.Substep_archive_log_directories, func(streams step.OutStreams) error {
			called++
			return nil
		})

		substepStore.Status = idl.Status_unknown_status

		s.Run(idl.Substep_shutdown_target_cluster, func(streams step.OutStreams) error {
			t.Error("expected substep to not be called")
			return nil
		})

		s.Run(idl.Substep_delete_backupdir, func(streams step.OutStreams) error {
			called++
			return nil
		})

		substepStore.Status = idl.Status_unknown_status

		s.Run(idl.Substep_delete_segment_statedirs, func(streams step.OutStreams) error {
			called++
			return nil
		})

		substepStore.Status = idl.Status_unknown_status

		expectedCalls := 3
		if called != expectedCalls {
			t.Errorf("called %d substeps, expected %d", called, expectedCalls)
		}
	})
}

func TestHasStarted(t *testing.T) {
	stateDir, err := os.MkdirTemp("", "")
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

		hasStarted, err := step.HasStarted(idl.Step_initialize)
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

		hasStarted, err := step.HasStarted(idl.Step_initialize)
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

		hasStarted, err := step.HasStarted(idl.Step_finalize)
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
			idl.Step_initialize, idl.Substep_backup_target_master, idl.Status_complete)
		testutils.MustWriteToFile(t, path, jsonContent)

		hasStarted, err := step.HasStarted(idl.Step_initialize)
		if err != nil {
			t.Errorf("HasStarted returned error %+v", err)
		}

		if !hasStarted {
			t.Errorf("expected substep to not have been run")
		}
	})
}

func TestHasRun(t *testing.T) {
	stateDir, err := os.MkdirTemp("", "")
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
			status:      idl.Status_running,
		},
		{
			description: "returns true when substep has completed",
			status:      idl.Status_complete,
		},
		{
			description: "returns true when substep has errored",
			status:      idl.Status_failed,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			store, err := step.NewSubstepFileStore()
			if err != nil {
				t.Fatalf("step.NewSubstepStore returned error %+v", err)
			}

			err = store.Write(idl.Step_initialize, idl.Substep_saving_source_cluster_config, c.status)
			if err != nil {
				t.Errorf("store.Write returned error %+v", err)
			}

			hasRun, err := step.HasRun(idl.Step_initialize, idl.Substep_saving_source_cluster_config)
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

		hasRun, err := step.HasRun(idl.Step_initialize, idl.Substep_saving_source_cluster_config)
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

		hasRun, err := step.HasRun(idl.Step_initialize, idl.Substep_saving_source_cluster_config)
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

		hasRan, err := step.HasRun(idl.Step_finalize, idl.Substep_saving_source_cluster_config)
		if err != nil {
			t.Errorf("HasRun returned error %+v", err)
		}

		if hasRan {
			t.Errorf("expected substep to not have been run")
		}
	})
}

func TestHasCompleted(t *testing.T) {
	stateDir, err := os.MkdirTemp("", "")
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
			status:      idl.Status_running,
			expected:    false,
		},
		{
			description: "returns true when substep has completed",
			status:      idl.Status_complete,
			expected:    true,
		},
		{
			description: "returns false when substep has errored",
			status:      idl.Status_failed,
			expected:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			store, err := step.NewSubstepFileStore()
			if err != nil {
				t.Fatalf("step.NewSubstepStore returned error %+v", err)
			}

			err = store.Write(idl.Step_initialize, idl.Substep_start_agents, c.status)
			if err != nil {
				t.Errorf("store.Write returned error %+v", err)
			}

			hasRun, err := step.HasCompleted(idl.Step_initialize, idl.Substep_start_agents)
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

		hasRun, err := step.HasCompleted(idl.Step_initialize, idl.Substep_start_agents)
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

		hasRun, err := step.HasCompleted(idl.Step_initialize, idl.Substep_start_agents)
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

		hasRan, err := step.HasCompleted(idl.Step_finalize, idl.Substep_start_agents)
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
		s := step.New(idl.Step_initialize, nil, nil, streams)

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
		s := step.New(idl.Step_initialize, nil, nil, streams)

		err := s.Finish()
		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}

func TestStepErr(t *testing.T) {
	t.Run("returns nil when substep did not fail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_complete,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return nil
		})

		err := s.Err()
		if err != nil {
			t.Errorf("unexpected error %#v", err)
		}
	})

	t.Run("does not set next action when error is not next action", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_failed,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		expected := os.ErrPermission
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return expected
		})

		err := s.Err()
		_, ok := status.FromError(err)
		if ok {
			t.Fatalf("got gRPC status error %#v, want %#v", err, expected)
		}
	})

	t.Run("sets next action when error is next action", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_failed,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		expected := utils.NewNextActionErr(os.ErrPermission, "change permissions to gpadmin")
		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return expected
		})

		err := s.Err()
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("got gRPC status error %#v, want %#v", err, expected)
		}

		for _, detail := range st.Details() {
			switch msg := detail.(type) {
			case *idl.NextActions:
				if msg.GetNextActions() != expected.NextAction {
					t.Fatalf("got %q want %q", msg.GetNextActions(), expected.NextAction)
				}
			default:
				t.Fatalf("expected details to contain NextActionErr")
			}
		}
	})

	t.Run("appends next actions when error is a list of errors containing next actions", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_running,
			}}})
		server.EXPECT().
			Send(&idl.Message{Contents: &idl.Message_Status{Status: &idl.SubstepStatus{
				Step:   idl.Substep_saving_source_cluster_config,
				Status: idl.Status_failed,
			}}})

		s := step.New(idl.Step_initialize, server, &TestSubstepStore{}, &testutils.DevNullWithClose{})

		expected1 := utils.NewNextActionErr(os.ErrPermission, "change permissions to gpadmin")
		expected2 := utils.NewNextActionErr(os.ErrDeadlineExceeded, "stop and rerun")
		errs := errorlist.Errors{
			errors.New("ahhh"),
			expected1,
			errors.New("oops"),
			expected2,
		}

		s.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return errs
		})

		err := s.Err()
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("got gRPC status error %#v, want %#v", err, expected1)
		}

		for _, detail := range st.Details() {
			switch msg := detail.(type) {
			case *idl.NextActions:
				expectedText := strings.Join([]string{expected1.NextAction, expected2.NextAction}, "\n")
				if msg.GetNextActions() != expectedText {
					t.Fatalf("got %q want %q", msg.GetNextActions(), expectedText)
				}
			default:
				t.Fatalf("expected details to contain NextActionErr")
			}
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
