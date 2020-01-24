package step_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
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

	t.Run("bubbles up any read failures", func(t *testing.T) {
		_, err := fs.Read(idl.Substep_CHECK_UPGRADE)

		if !os.IsNotExist(err) {
			t.Errorf("returned error %#v, want ErrNotExist", err)
		}
	})

	err = ioutil.WriteFile(path, []byte("{}"), 0600)
	if err != nil {
		t.Fatalf("writing initial status file: %v", err)
	}

	t.Run("reads the same status that was written", func(t *testing.T) {
		substep := idl.Substep_CHECK_UPGRADE
		expected := idl.Status_COMPLETE

		err := fs.Write(substep, expected)
		if err != nil {
			t.Fatalf("Write() returned error %#v", err)
		}

		status, err := fs.Read(substep)
		if err != nil {
			t.Errorf("Read() returned error %#v", err)
		}
		if status != expected {
			t.Errorf("read %v, want %v", status, expected)
		}
	})

	t.Run("returns unknown status if substep has not been written", func(t *testing.T) {
		err = ioutil.WriteFile(path, []byte("{}"), 0600)
		if err != nil {
			t.Fatalf("clearing status file: %v", err)
		}

		status, err := fs.Read(idl.Substep_INIT_TARGET_CLUSTER)
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
		if err := fs.Write(substep, status); err != nil {
			t.Fatalf("Write(): %+v", err)
		}

		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("opening file: %+v", err)
		}
		defer f.Close()

		dec := json.NewDecoder(f)
		raw := make(map[string]string)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("decoding statuses: %+v", err)
		}

		key := substep.String()
		if raw[key] != status.String() {
			t.Errorf("status[%q] = %q, want %q", key, raw[key], status.String())
		}
	})
}
