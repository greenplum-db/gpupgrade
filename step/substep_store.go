// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

type SubstepStore interface {
	Read(idl.Step, idl.Substep) (idl.Status, error)
	Write(idl.Step, idl.Substep, idl.Status) error
}

// SubstepFileStore implements SubstepStore by providing persistent storage on disk.
type SubstepFileStore struct {
	path string
}

func NewSubstepFileStore() (*SubstepFileStore, error) {
	path, err := utils.GetJSONFile(utils.GetStateDir(), SubstepsFileName)
	if err != nil {
		return &SubstepFileStore{}, xerrors.Errorf("read %q: %w", SubstepsFileName, err)
	}

	return &SubstepFileStore{path}, nil
}

func NewSubstepStoreUsingFile(path string) *SubstepFileStore {
	return &SubstepFileStore{path}
}

type prettyMap = map[string]map[string]PrettyStatus

// PrettyStatus exists only to write a string description of idl.Status to
// the JSON representation, instead of an integer.
type PrettyStatus struct {
	idl.Status
}

func (p PrettyStatus) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *PrettyStatus) UnmarshalText(buf []byte) error {
	name := string(buf)

	val, ok := idl.Status_value[name]
	if !ok {
		return fmt.Errorf("unknown substep name %q", name)
	}

	p.Status = idl.Status(val)
	return nil
}

func (f *SubstepFileStore) load() (prettyMap, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}

	var substeps prettyMap
	err = json.Unmarshal(data, &substeps)
	if err != nil {
		return nil, err
	}

	return substeps, nil
}

func (f *SubstepFileStore) ReadStep(step idl.Step) (map[string]PrettyStatus, error) {
	steps, err := f.load()
	if err != nil {
		return nil, err
	}

	sectionMap, ok := steps[step.String()]
	if !ok {
		return nil, nil
	}

	return sectionMap, nil
}

func (f *SubstepFileStore) Read(step idl.Step, substep idl.Substep) (idl.Status, error) {
	sectionMap, err := f.ReadStep(step)
	if err != nil {
		return idl.Status_unknown_status, err
	}

	status, ok := sectionMap[substep.String()]
	if !ok {
		return idl.Status_unknown_status, nil
	}

	return status.Status, nil
}

// Write atomically updates the status file.
// Load the latest values from the filesystem, rather than storing
// in-memory on a struct to avoid having two sources of truth.
func (f *SubstepFileStore) Write(step idl.Step, substep idl.Substep, status idl.Status) (err error) {
	steps, err := f.load()
	if err != nil {
		return err
	}

	if _, ok := steps[step.String()]; !ok {
		steps[step.String()] = make(map[string]PrettyStatus)
	}
	steps[step.String()][substep.String()] = PrettyStatus{status}

	data, err := json.MarshalIndent(steps, "", "  ") // pretty print JSON
	if err != nil {
		return err
	}

	return utils.AtomicallyWrite(f.path, data)
}
