// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestFileStore(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	path := filepath.Join(tmpDir, "status.json")
	fs := step.NewFileStore(path)

	const step = idl.Step_INITIALIZE

	t.Run("bubbles up any read failures", func(t *testing.T) {
		_, err := fs.Read(step, idl.Substep_CHECK_UPGRADE)

		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v, want ErrNotExist", err)
		}

		ran, err := fs.HasStepRun(step)
		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v, want ErrNotExist", err)
		}
		if ran {
			t.Errorf("expected step to not be run")
		}
	})

	t.Run("reads the same status that was written", func(t *testing.T) {
		clear(t, path)

		substep := idl.Substep_CHECK_UPGRADE
		expected := idl.Status_COMPLETE

		err := fs.Write(step, substep, expected)
		if err != nil {
			t.Fatalf("Write() returned error %#v", err)
		}

		status, err := fs.Read(step, substep)
		if err != nil {
			t.Errorf("Read() returned error %#v", err)
		}
		if status != expected {
			t.Errorf("read %v, want %v", status, expected)
		}

		ran, err := fs.HasStepRun(step)
		if err != nil {
			t.Errorf("HasStepRun() returned error %#v", err)
		}
		if !ran {
			t.Errorf("expected step to be run")
		}

		ran, err = fs.HasStepRun(idl.Step_UNKNOWN_STEP)
		if err != nil {
			t.Errorf("HasStepRun() returned error %#v", err)
		}
		if ran {
			t.Errorf("expected unknown step to not be run")
		}
	})

	t.Run("can write to the same substep in different steps", func(t *testing.T) {
		clear(t, path)

		substep := idl.Substep_CHECK_UPGRADE
		entries := []struct {
			Step   idl.Step
			Status idl.Status
		}{
			{Step: idl.Step_INITIALIZE, Status: idl.Status_FAILED},
			{Step: idl.Step_EXECUTE, Status: idl.Status_COMPLETE},
		}

		for _, e := range entries {
			err := fs.Write(e.Step, substep, e.Status)
			if err != nil {
				t.Fatalf("Write(%q, %v, %v) returned error %+v",
					e.Step, substep, e.Status, err)
			}
		}

		for _, e := range entries {
			status, err := fs.Read(e.Step, substep)
			if err != nil {
				t.Errorf("Read(%q, %v) returned error %#v", e.Step, substep, err)
			}
			if status != e.Status {
				t.Errorf("Read(%q, %v) = %v, want %v", e.Step, substep,
					status, e.Status)
			}
		}
	})

	t.Run("returns unknown status if requested step has not been written", func(t *testing.T) {
		clear(t, path)

		status, err := fs.Read(step, idl.Substep_INIT_TARGET_CLUSTER)
		if err != nil {
			t.Errorf("Read() returned error %#v", err)
		}

		expected := idl.Status_UNKNOWN_STATUS
		if status != expected {
			t.Errorf("read %v, want %v", status, expected)
		}
	})

	t.Run("returns unknown status if substep was not written to the requested step", func(t *testing.T) {
		clear(t, path)

		err := fs.Write(step, idl.Substep_CHECK_UPGRADE, idl.Status_FAILED)
		if err != nil {
			t.Fatalf("Write() returned error %+v", err)
		}

		status, err := fs.Read(step, idl.Substep_INIT_TARGET_CLUSTER)
		if err != nil {
			t.Errorf("Read() returned error %#v", err)
		}

		expected := idl.Status_UNKNOWN_STATUS
		if status != expected {
			t.Errorf("read %v, want %v", status, expected)
		}
	})

	t.Run("returns unknown status if substep was written to a different step", func(t *testing.T) {
		clear(t, path)

		err := fs.Write(idl.Step_FINALIZE, idl.Substep_INIT_TARGET_CLUSTER, idl.Status_FAILED)
		if err != nil {
			t.Fatalf("Write() returned error %+v", err)
		}

		status, err := fs.Read(step, idl.Substep_INIT_TARGET_CLUSTER)
		if err != nil {
			t.Errorf("Read() returned error %#v", err)
		}

		expected := idl.Status_UNKNOWN_STATUS
		if status != expected {
			t.Errorf("read %v, want %v", status, expected)
		}
	})

	t.Run("uses human-readable serialization", func(t *testing.T) {
		substep := idl.Substep_INIT_TARGET_CLUSTER
		status := idl.Status_FAILED
		if err := fs.Write(step, substep, status); err != nil {
			t.Fatalf("Write(): %+v", err)
		}

		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening file: %+v", err)
		}
		defer f.Close()

		dec := json.NewDecoder(f)
		raw := make(map[string]map[string]string)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("decoding statuses: %+v", err)
		}

		key := substep.String()
		if raw[step.String()][key] != status.String() {
			t.Errorf("status[%q][%q] = %q, want %q", step, key, raw[step.String()][key], status.String())
		}
	})
}

// clear writes an empty JSON map to the given FileStore backing path.
func clear(t *testing.T, path string) {
	t.Helper()

	testutils.MustWriteToFile(t, path, "{}")
}
