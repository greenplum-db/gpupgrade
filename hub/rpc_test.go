// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub_test

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/idl"
)

func TestExecuteRPC(t *testing.T) {
	t.Run("executes multiple requests", func(t *testing.T) {
		agentConns := []*idl.Connection{
			{Hostname: "mdw"},
			{Hostname: "sdw"},
		}

		hosts := make(chan string, len(agentConns))
		request := func(conn *idl.Connection) error {
			hosts <- conn.Hostname
			return nil
		}

		err := hub.ExecuteRPC(agentConns, request)
		if err != nil {
			t.Errorf("ExecuteRPC returned error %+v", err)
		}

		close(hosts)

		var actual []string
		for host := range hosts {
			actual = append(actual, host)
		}

		expected := []string{"mdw", "sdw"}
		sort.Strings(actual)
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("got %v want %v", actual, expected)
		}
	})

	t.Run("bubbles up errors", func(t *testing.T) {
		agentConns := []*idl.Connection{
			{Hostname: "mdw"},
			{Hostname: "sdw"},
		}

		expected := errors.New("permission denied")
		request := func(conn *idl.Connection) error {
			if conn.Hostname == "mdw" {
				return expected
			}

			return nil
		}

		err := hub.ExecuteRPC(agentConns, request)

		if !errors.Is(err, expected) {
			t.Errorf("got error %#v, want %#v", err, expected)
		}
	})
}
