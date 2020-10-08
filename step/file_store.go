// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

type Store interface {
	HasStepRun(idl.Step) (bool, error)
	Read(idl.Step, idl.Substep) (idl.Status, error)
	Write(idl.Step, idl.Substep, idl.Status) error
}

// FileStore implements step.Store by providing persistent storage on disk.
type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path}
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

func (f *FileStore) load() (prettyMap, error) {
	data, err := ioutil.ReadFile(f.path)
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

func (f *FileStore) Read(step idl.Step, substep idl.Substep) (idl.Status, error) {
	steps, err := f.load()
	if err != nil {
		return idl.Status_UNKNOWN_STATUS, err
	}

	substeps, ok := steps[step.String()]
	if !ok {
		return idl.Status_UNKNOWN_STATUS, nil
	}

	status, ok := substeps[substep.String()]
	if !ok {
		return idl.Status_UNKNOWN_STATUS, nil
	}

	return status.Status, nil
}

func (f *FileStore) HasStepRun(step idl.Step) (bool, error) {
	steps, err := f.load()
	if err != nil {
		return false, err
	}

	_, ok := steps[step.String()]
	if !ok {
		return false, nil
	}

	return true, nil
}

// Write atomically updates the status file.
// Load the latest values from the filesystem, rather than storing
// in-memory on a struct to avoid having two sources of truth.
func (f *FileStore) Write(step idl.Step, substep idl.Substep, status idl.Status) (err error) {
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
