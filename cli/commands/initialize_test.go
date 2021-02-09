//  Copyright (c) 2017-2021 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/greenplum-db/gpupgrade/idl"
)

func TestParsePorts(t *testing.T) {
	cases := []struct {
		input    string
		expected []uint32
	}{
		{"", []uint32(nil)},
		{"1", []uint32{1}},
		{"1,3,5", []uint32{1, 3, 5}},
		/* ranges */
		{"1-5", []uint32{1, 2, 3, 4, 5}},
		{"1-5,6-10", []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"1-5,10,12,15-15", []uint32{1, 2, 3, 4, 5, 10, 12, 15}},
	}

	for _, c := range cases {
		actual, err := parsePorts(c.input)
		if err != nil {
			t.Errorf("parsePorts(%q) returned error %#v", c.input, err)
		}
		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("parsePorts(%q) returned %v, want %v", c.input, actual, c.expected)
		}
	}

	errorCases := []string{
		"1, 3, 5",
		"sdklfjds",
		"-1",
		"5-1",
		"1--5",
		"1-3-5",
		"1,,2",
		"1,a",
		"1-a",
		"a-1",
		"900000",
		"1-900000",
		"900000-1000000",
		",1",
	}

	for _, c := range errorCases {
		actual, err := parsePorts(c)
		if err == nil {
			t.Errorf("parsePorts(%q) returned %v instead of an error", c, actual)
		}
	}
}

func TestIsLinkMode(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		expected bool
	}{
		{
			name:     "parses copy",
			mode:     "copy",
			expected: false,
		},
		{
			name:     "parses link",
			mode:     "link",
			expected: true,
		},
		{
			name:     "parses capitalizations",
			mode:     "LiNk",
			expected: true,
		},
		{
			name:     "trims spaces",
			mode:     " link  \t",
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			linkMode, err := isLinkMode(c.mode)
			if err != nil {
				t.Errorf("unexpected error %#v", err)
			}

			if linkMode != c.expected {
				t.Errorf("got %t want %t", linkMode, c.expected)
			}
		})
	}

	errCases := []struct {
		name string
		mode string
	}{
		{
			name: "empty string",
			mode: "",
		},
		{
			name: "invalid mode",
			mode: "depeche",
		},
		{
			name: "errors on numbers",
			mode: "1",
		},
	}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			linkMode, err := isLinkMode(c.mode)
			if err == nil {
				t.Errorf("isLinkMode(%q) returned %v instead of an error", c.mode, err)
			}

			if linkMode != false {
				t.Errorf("got linkMode %t want %t", linkMode, false)
			}
		})
	}
}

func TestAddFlags(t *testing.T) {
	t.Run("sets flags to correct value and marks them as changed", func(t *testing.T) {
		var name string
		var port int
		var isSet bool
		var unsetFlag string
		cmd := cobra.Command{}
		cmd.Flags().StringVar(&name, "name", "", "")
		cmd.Flags().IntVar(&port, "port", 0, "")
		cmd.Flags().BoolVar(&isSet, "is-set", false, "")
		cmd.Flags().StringVar(&unsetFlag, "unset-flag", "", "")

		flags := map[string]string{
			"name":   "value",
			"port":   "123",
			"is-set": "true",
		}

		err := addFlags(&cmd, flags)
		if err != nil {
			t.Errorf("addFlags returned error %+v", err)
		}

		// verify string flags
		if name != flags["name"] {
			t.Errorf("got %q want %q", name, flags["name"])
		}

		// verify int flags
		expectedPort, err := strconv.Atoi(flags["port"])
		if err != nil {
			t.Errorf("Atoi returned error: %+v", err)
		}

		if port != expectedPort {
			t.Errorf("got %d want %d", port, expectedPort)
		}

		// verify bool flags
		expectedBool, err := strconv.ParseBool(flags["is-set"])
		if err != nil {
			t.Errorf("ParseBool returned error: %+v", err)
		}

		if isSet != expectedBool {
			t.Errorf("got %t want %t", isSet, expectedBool)
		}

		// verify flags have been changed
		cmd.Flags().Visit(func(flag *pflag.Flag) {
			if !flag.Changed {
				t.Errorf("expected flag %q to be changed", flag.Name)
			}
		})

		// verify unset flags have not been changed
		flag := cmd.Flag("unset-flag")
		if flag.Changed {
			t.Errorf("expected unset flag %q to not be changed", flag.Name)
		}
	})

	t.Run("errors when adding unknown parameter", func(t *testing.T) {
		flags := map[string]string{
			"unknown": "value",
		}

		err := addFlags(&cobra.Command{}, flags)
		if err == nil {
			t.Errorf("expected error %#v got nil", err)
		}
	})
}

func TestInitializeWarningMessageIfAny(t *testing.T) {
	cases := []struct {
		name     string
		input    idl.InitializeResponse
		expected string
	}{
		{
			name: "both standby and mirror does not exist",
			input: idl.InitializeResponse{
				HasMirrors: false,
				HasStandby: false,
			},
			expected: fmt.Sprintf(InitializeWarningMessage, "standby and mirror segments"),
		},
		{
			name: "only mirrors does not exist",
			input: idl.InitializeResponse{
				HasMirrors: false,
				HasStandby: true,
			},
			expected: fmt.Sprintf(InitializeWarningMessage, "mirror segments"),
		},
		{
			name: "only standby does not exist",
			input: idl.InitializeResponse{
				HasMirrors: true,
				HasStandby: false,
			},
			expected: fmt.Sprintf(InitializeWarningMessage, "standby"),
		},
		{
			name: "both standby and mirrors exist",
			input: idl.InitializeResponse{
				HasMirrors: true,
				HasStandby: true,
			},
			expected: "",
		},
	}

	for _, c := range cases {
		resultMessage := InitializeWarningMessageIfAny(c.input)
		if resultMessage != c.expected {
			t.Errorf("got %q, want %q", resultMessage, c.expected)
		}
	}
}
