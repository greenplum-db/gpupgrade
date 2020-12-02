// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

var expectedHosts = []string{"sdw1", "sdw2"}

const version5X = "5.0.0"
const version6X = "6.0.0"

var expected = errors.New("oops")

type Versions struct {
	hubVersion   string
	agentVersion string
	hubErr       error
	agentErr     error
}

func (t *Versions) Local() (string, error) {
	return t.hubVersion, t.hubErr
}

func (t *Versions) Remote(host string) (string, error) {
	return t.agentVersion, t.agentErr
}

func TestEnsureVersionsMatch(t *testing.T) {
	testlog.SetupLogger()

	t.Run("ensure versions match", func(t *testing.T) {
		err := EnsureVersionsMatch(expectedHosts, &Versions{hubVersion: version6X, agentVersion: version6X})
		if err != nil {
			t.Errorf("unexpected errr %#v", err)
		}
	})

	t.Run("errors when failing to get version on the hub", func(t *testing.T) {
		err := EnsureVersionsMatch(expectedHosts, &Versions{hubErr: expected})
		if !errors.Is(err, expected) {
			t.Errorf("got %v want %v", err, expected)
		}
	})

	t.Run("errors when failing to get version on the agents", func(t *testing.T) {
		err := EnsureVersionsMatch(expectedHosts, &Versions{hubVersion: version6X, agentErr: expected})
		var expected errorlist.Errors
		if !errors.As(err, &expected) {
			t.Fatalf("got type %T, want type %T", err, expected)
		}

		if !reflect.DeepEqual(err, expected) {
			t.Fatalf("got err %#v, want %#v", err, expected)
		}
	})

	t.Run("reports version mismatch between hub and agent", func(t *testing.T) {
		err := EnsureVersionsMatch(expectedHosts, &Versions{hubVersion: version6X, agentVersion: version5X})
		if err == nil {
			t.Errorf("expected an error")
		}

		expected := MismatchedVersions{version5X: expectedHosts}
		if !strings.HasSuffix(err.Error(), expected.String()) {
			t.Error("expected error to contain mismatched agents")
			t.Logf("got err: %s", err)
			t.Logf("want suffix: %s", expected)
		}
	})
}
