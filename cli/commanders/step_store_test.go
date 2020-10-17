//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils"
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

	store, err := commanders.NewStepStore()
	if err != nil {
		t.Fatalf("NewStepStore failed: %v", err)
	}

	t.Run("write persists the step status", func(t *testing.T) {
		expected := idl.Status_RUNNING
		err := store.Write(idl.Step_INITIALIZE, expected)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		status, err := store.Read(idl.Step_INITIALIZE)
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

		store, err := commanders.NewStepStore()
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("got %T, want %T", err, pathErr)
		}

		expected := &commanders.StepStore{}
		if !reflect.DeepEqual(store, expected) {
			t.Errorf("got %v want %v", store, expected)
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

		store, err := commanders.NewStepStore()
		if err != nil {
			t.Fatalf("NewStepStore failed: %v", err)
		}

		// remove state directory
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatalf("removing temp state directory: %v", err)
		}

		err = store.Write(idl.Step_INITIALIZE, idl.Status_RUNNING)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("returned error type %T want %T", err, pathErr)
		}

		status, err := store.Read(idl.Step_INITIALIZE)
		if !errors.As(err, &pathErr) {
			t.Errorf("returned error type %T want %T", err, pathErr)
		}

		expected := idl.Status_UNKNOWN_STATUS
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})
}
