package hub

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/testutils"
)

func TestConfig(t *testing.T) {
	// "stream" refers to the io.Writer/Reader interfaces.
	t.Run("saves itself to the provided stream", func(t *testing.T) {
		source, target := testutils.CreateMultinodeSampleClusterPair("/tmp")
		targetInitializeConfig := InitializeConfig{Master: greenplum.SegConfig{Hostname: "mdw"}}
		original := &Config{source, target, targetInitializeConfig, 12345, 54321, false}

		buf := new(bytes.Buffer)
		err := original.Save(buf)
		if err != nil {
			t.Errorf("Save() returned error %+v", err)
		}

		// Save the buffer contents for later debugging (otherwise they'll be
		// consumed by Load()).
		contents := buf.String()

		duplicate := new(Config)
		err = duplicate.Load(buf)
		if err != nil {
			t.Errorf("Load() returned error %+v", err)
		}

		if !reflect.DeepEqual(original, duplicate) {
			// XXX This error message is less useful than it could be, because
			// %#v doesn't expand struct pointers recursively.
			t.Errorf("save-load cycle resulted in %#v, want %#v", duplicate, original)
			t.Logf("buffer contents:\n%s", contents)
		}
	})
}
