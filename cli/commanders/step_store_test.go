//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestStepStore(t *testing.T) {
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

	stepStore, err := commanders.NewStepStore()
	if err != nil {
		t.Fatalf("NewStepStore failed: %v", err)
	}

	t.Run("write persists the step status", func(t *testing.T) {
		expected := idl.Status_RUNNING
		err := stepStore.Write(idl.Step_INITIALIZE, expected)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		status, err := stepStore.Read(idl.Step_INITIALIZE)
		if err != nil {
			t.Errorf("Read failed %#v", err)
		}

		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("cannot create a new step store if state directory does not exist", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", "/does/not/exist")
		defer resetEnv()

		stepStore, err := commanders.NewStepStore()
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("got %T, want %T", err, pathErr)
		}

		expected := &commanders.StepStore{}
		if !reflect.DeepEqual(stepStore, expected) {
			t.Errorf("got %v want %v", stepStore, expected)
		}
	})

	t.Run("write errors and read errors with unknown status when failing to get the steps status file", func(t *testing.T) {
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

		stepStore, err := commanders.NewStepStore()
		if err != nil {
			t.Fatalf("NewStepStore failed: %v", err)
		}

		// remove state directory
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatalf("removing temp state directory: %v", err)
		}

		err = stepStore.Write(idl.Step_INITIALIZE, idl.Status_RUNNING)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("returned error type %T want %T", err, pathErr)
		}

		status, err := stepStore.Read(idl.Step_INITIALIZE)
		if !errors.As(err, &pathErr) {
			t.Errorf("returned error type %T want %T", err, pathErr)
		}

		expected := idl.Status_UNKNOWN_STATUS
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("HasStatus errors with false when failing to get the steps status file", func(t *testing.T) {
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

		stepStore, err := commanders.NewStepStore()
		if err != nil {
			t.Fatalf("NewStepStore failed: %v", err)
		}

		// remove state directory
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatalf("removing temp state directory: %v", err)
		}

		called := false
		check := func(status idl.Status) bool {
			called = true
			return true
		}

		hasStatus, err := stepStore.HasStatus(idl.Step_INITIALIZE, check)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("returned error type %T want %T", err, pathErr)
		}

		if hasStatus {
			t.Error("expected hasStatus to be false")
		}

		if called {
			t.Error("expected check function to not be called")
		}
	})

	t.Run("HasStepStarted returns true if a step's status is running, complete, or failed", func(t *testing.T) {
		statuses := []idl.Status{idl.Status_RUNNING, idl.Status_COMPLETE, idl.Status_FAILED}
		for _, status := range statuses {
			err := stepStore.Write(idl.Step_INITIALIZE, status)
			if err != nil {
				t.Errorf("Write failed %#v", err)
			}

			started, err := stepStore.HasStepStarted(idl.Step_INITIALIZE)
			if err != nil {
				t.Errorf("HasStepStarted failed %#v", err)
			}

			if !started {
				t.Error("expected step to have been started")
			}
		}
	})

	t.Run("HasStepStarted returns false if a step has not started", func(t *testing.T) {
		started, err := stepStore.HasStepStarted(idl.Step_UNKNOWN_STEP)
		if err != nil {
			t.Errorf("HasStepStarted failed %#v", err)
		}

		if started {
			t.Error("expected step to have not been started")
		}
	})

	t.Run("HasStepCompleted returns true if a step's status is complete", func(t *testing.T) {
		err := stepStore.Write(idl.Step_INITIALIZE, idl.Status_COMPLETE)
		if err != nil {
			t.Errorf("Write failed %#v", err)
		}

		completed, err := stepStore.HasStepCompleted(idl.Step_INITIALIZE)
		if err != nil {
			t.Errorf("HasStepCompleted failed %#v", err)
		}

		if !completed {
			t.Error("expected step to have been completed")
		}
	})

	t.Run("HasStepCompleted returns false if a step's status is not complete", func(t *testing.T) {
		statuses := []idl.Status{idl.Status_RUNNING, idl.Status_FAILED, idl.Status_SKIPPED, idl.Status_UNKNOWN_STATUS}
		for _, status := range statuses {
			err := stepStore.Write(idl.Step_INITIALIZE, status)
			if err != nil {
				t.Errorf("Write failed %#v", err)
			}

			completed, err := stepStore.HasStepCompleted(idl.Step_INITIALIZE)
			if err != nil {
				t.Errorf("HasStepCompleted failed %#v", err)
			}

			if completed {
				t.Error("expected step to have not been completed")
			}
		}
	})
}

func TestValidateStep(t *testing.T) {
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

	stepStore, err := commanders.NewStepStore()
	if err != nil {
		t.Fatalf("NewStepStore failed: %v", err)
	}

	type stepStatus struct {
		step   idl.Step
		status idl.Status
	}

	errorCases := []struct {
		name               string
		currentStep        idl.Step
		preconditions      []stepStatus
		expectedNextAction string
	}{
		// error cases when current step is initialize
		{
			"fails when initialize is run but execute has already started",
			idl.Step_INITIALIZE,
			[]stepStatus{{step: idl.Step_EXECUTE, status: idl.Status_RUNNING}},
			commanders.RunExecute,
		},
		{
			"fails when initialize is run but finalize has started",
			idl.Step_INITIALIZE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_COMPLETE},
				{step: idl.Step_FINALIZE, status: idl.Status_RUNNING}},
			commanders.RunFinalize,
		},
		{
			"fails when initialize is run but revert has started",
			idl.Step_INITIALIZE,
			[]stepStatus{{step: idl.Step_REVERT, status: idl.Status_RUNNING}},
			commanders.RunRevert,
		},
		// error cases when current step is execute
		{
			"fails when execute is run before initialize has completed",
			idl.Step_EXECUTE,
			[]stepStatus{},
			commanders.RunInitialize,
		},
		{
			"fails when execute is run but finalize has started",
			idl.Step_EXECUTE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_FINALIZE, status: idl.Status_RUNNING}},
			commanders.RunFinalize,
		},
		{
			"fails when execute is run but revert has started",
			idl.Step_EXECUTE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_REVERT, status: idl.Status_RUNNING}},
			commanders.RunRevert,
		},
		// error cases when current step is finalize
		{
			"fails when finalize is run before initialize has completed",
			idl.Step_FINALIZE,
			[]stepStatus{},
			commanders.RunInitialize,
		},
		{
			"fails when finalize is run and execute has not started",
			idl.Step_FINALIZE,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE}},
			commanders.RunExecute,
		},
		{
			"fails when finalize is run but revert has started",
			idl.Step_FINALIZE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_FAILED},
				{step: idl.Step_REVERT, status: idl.Status_FAILED}},
			commanders.RunRevert,
		},
		// error cases when current step is revert
		{
			"fails when revert is run before initialize has completed",
			idl.Step_REVERT,
			[]stepStatus{},
			commanders.RunInitialize,
		},
		{
			"fails when revert is run but finalize has already been started",
			idl.Step_REVERT,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_COMPLETE},
				{step: idl.Step_FINALIZE, status: idl.Status_RUNNING},
			},
			commanders.RunFinalize,
		},
	}

	for _, c := range errorCases {
		t.Run(c.name, func(t *testing.T) {
			clearStepStore(t)

			for _, condition := range c.preconditions {
				mustWriteStatus(t, stepStore, condition.step, condition.status)
			}

			err = stepStore.ValidateStep(c.currentStep)
			var nextActionsErr utils.NextActionErr
			if !errors.As(err, &nextActionsErr) {
				t.Errorf("got %T, want %T", err, nextActionsErr)
			}

			if nextActionsErr.NextAction != c.expectedNextAction {
				t.Errorf("got %q want %q", nextActionsErr.NextAction, c.expectedNextAction)
			}
		})
	}

	cases := []struct {
		name          string
		currentStep   idl.Step
		preconditions []stepStatus
	}{
		// positive cases when current step is initialize
		{
			"can run initialize",
			idl.Step_INITIALIZE,
			[]stepStatus{},
		},
		{
			"can run initialize after initialize is running",
			idl.Step_INITIALIZE,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_RUNNING}},
		},
		{
			"can run initialize after initialize has failed",
			idl.Step_INITIALIZE,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_FAILED}},
		},
		{
			"can run initialize after initialize is completed",
			idl.Step_INITIALIZE,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE}},
		},
		// positive cases when current step is execute
		{
			"can run execute after initialize has completed",
			idl.Step_EXECUTE,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE}},
		},
		{
			"can run execute after execute has failed",
			idl.Step_EXECUTE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_FAILED}},
		},
		// positive cases when current step is finalize
		{
			"can run finalize after execute has completed",
			idl.Step_FINALIZE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_COMPLETE},
			},
		},
		{
			"can run finalize after finalize has failed",
			idl.Step_FINALIZE,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_COMPLETE},
				{step: idl.Step_FINALIZE, status: idl.Status_FAILED}},
		},
		// positive cases when current step is revert
		{
			"can run revert after initialize has started",
			idl.Step_REVERT,
			[]stepStatus{{step: idl.Step_INITIALIZE, status: idl.Status_RUNNING}},
		},
		{
			"can run revert after execute has started",
			idl.Step_REVERT,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_COMPLETE},
				{step: idl.Step_EXECUTE, status: idl.Status_RUNNING},
			},
		},
		{
			"can run revert after revert has failed",
			idl.Step_REVERT,
			[]stepStatus{
				{step: idl.Step_INITIALIZE, status: idl.Status_FAILED},
				{step: idl.Step_REVERT, status: idl.Status_FAILED}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearStepStore(t)

			for _, condition := range c.preconditions {
				mustWriteStatus(t, stepStore, condition.step, condition.status)
			}

			err = stepStore.ValidateStep(c.currentStep)
			if err != nil {
				t.Errorf("unexpected err %#v", err)
			}
		})
	}
}

func clearStepStore(t *testing.T) {
	t.Helper()

	path := filepath.Join(utils.GetStateDir(), commanders.StepsFileName)
	testutils.MustWriteToFile(t, path, "{}")
}

func mustWriteStatus(t *testing.T, stepStore *commanders.StepStore, step idl.Step, status idl.Status) {
	t.Helper()

	err := stepStore.Write(step, status)
	if err != nil {
		t.Errorf("stepStore.Write returned error %+v", err)
	}
}
