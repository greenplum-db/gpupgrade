// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/xerrors"
)

type parameter struct {
	name  string
	value string
}

func ParseConfig(config io.Reader) (map[string]string, error) {
	flags := make(map[string]string)

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

		// For config file parameter names replace all underscores with dashes
		// such that the equivalent cobra command line flag can be found.
		name := strings.ReplaceAll(param.name, "_", "-")
		if _, ok := flags[name]; ok {
			return nil, xerrors.Errorf("parameter %q declared more than once", param.name)
		}

		flags[name] = param.value
	}

	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("scanning config: %w", err)
	}

	return flags, nil
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
	if value == "" {
		return parameter{}, xerrors.Errorf("no value found for parameter %q", name)
	}

	return parameter{name: name, value: value}, nil
}
