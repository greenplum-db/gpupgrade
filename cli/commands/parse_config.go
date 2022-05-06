// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

type parameter struct {
	name  string
	value string
}

// ParseConfig returns a validated map of flags from a gpupgrade config file.
func ParseConfig(config io.Reader) (map[string]string, error) {
	params, err := parseParams(config)
	if err != nil {
		return nil, err
	}

	// For config file parameter names replace all underscores with dashes
	// such that the equivalent cobra command line flag can be found.
	flags := make(map[string]string)
	for name, value := range params {
		flag := strings.ReplaceAll(name, "_", "-")
		flags[flag] = value
	}

	if err := checkConfig(flags); err != nil {
		return nil, err
	}

	return flags, nil
}

func parseParams(config io.Reader) (map[string]string, error) {
	params := make(map[string]string)

	scanner := bufio.NewScanner(config)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		param, err := parseLine(line)
		if err != nil {
			return nil, err
		}

		if _, ok := params[param.name]; ok {
			return nil, xerrors.Errorf("parameter %q declared more than once", param.name)
		}

		params[param.name] = param.value
	}

	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("scanning config: %w", err)
	}

	return params, nil
}

// parseLine allows one parameter per line with a required equal sign between
// the name and value. Comments begin with an "#" and can begin anywhere on the
// line.
func parseLine(line string) (parameter, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return parameter{}, xerrors.Errorf("parameter %q is not of the form name = value", line)
	}

	name := strings.TrimSpace(parts[0])

	value := strings.TrimSpace(parts[1])
	value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0]) // remove inline comments

	return parameter{name: name, value: value}, nil
}

func checkConfig(flags map[string]string) error {
	var err error
	for name, value := range flags {
		if value == "" {
			// To report the correct parameter to users use underscores when
			// referencing config file parameters.
			name = strings.ReplaceAll(name, "-", "_")
			err = errorlist.Append(err, xerrors.Errorf("no value found for parameter %q", name))
		}
	}
	return err
}
