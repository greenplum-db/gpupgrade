// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/hub"
)

func TestExecuteRPC(t *testing.T) {
	t.Run("executes multiple requests", func(t *testing.T) {
		agentConns := []*hub.Connection{
			{nil, nil, "mdw", nil},
			{nil, nil, "sdw", nil},
		}

		var hosts []string
		request := func(conn *hub.Connection) error {
			hosts = append(hosts, conn.Hostname)
			return nil
		}

		err := hub.ExecuteRPC(agentConns, request)
		if err != nil {
			t.Errorf("ExecuteRPC returned error %+v", err)
		}

		expected := []string{"mdw", "sdw"}
		sort.Strings(hosts)
		if !reflect.DeepEqual(hosts, expected) {
			t.Errorf("got %v want %v", hosts, expected)
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
		agentConns := []*hub.Connection{
			{nil, nil, "mdw", nil},
			{nil, nil, "sdw", nil},
		}

		expected := errors.New("permission denied")
		request := func(conn *hub.Connection) error {
			if conn.Hostname == "mdw" {
				return expected
			}

			return nil
		}

		err := hub.ExecuteRPC(agentConns, request)
		var multiErr *multierror.Error
		if !xerrors.As(err, &multiErr) {
			t.Fatalf("got error %#v, want type %T", err, multiErr)
		}

		if len(multiErr.Errors) != 1 {
			t.Errorf("received %d errors, want %d", len(multiErr.Errors), 1)
		}

		for _, err := range multiErr.Errors {
			if !xerrors.Is(err, expected) {
				t.Errorf("got error %#v, want %#v", expected, err)
			}
		}
	})
}
