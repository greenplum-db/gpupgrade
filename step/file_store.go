// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package step

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/google/renameio"
	"github.com/hashicorp/go-multierror"

	"github.com/greenplum-db/gpupgrade/idl"
)

type Store interface {
	Read(idl.Substep) (idl.Status, error)
	Write(idl.Substep, idl.Status) error
}

// FileStore implements step.Store by providing persistent storage on disk.
type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path}
}

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

func (f *FileStore) load() (map[string]idl.Status, error) {
	data, err := ioutil.ReadFile(f.path)
	if err != nil {
		return nil, err
	}

	var prettySubsteps map[string]PrettyStatus
	err = json.Unmarshal(data, &prettySubsteps)
	if err != nil {
		return nil, err
	}

	substeps := make(map[string]idl.Status)
	for k, v := range prettySubsteps {
		substeps[k] = v.Status
	}
	return substeps, nil
}

func (f *FileStore) Read(substep idl.Substep) (idl.Status, error) {
	steps, err := f.load()
	if err != nil {
		return idl.Status_UNKNOWN_STATUS, err
	}

	status, ok := steps[substep.String()]
	if !ok {
		return idl.Status_UNKNOWN_STATUS, nil
	}

	return status, nil
}

// Write atomically updates the status file.
// Load the latest values from the filesystem, rather than storing
// in-memory on a struct to avoid having two sources of truth.
func (f *FileStore) Write(substep idl.Substep, status idl.Status) (err error) {
	steps, err := f.load()
	if err != nil {
		return err
	}

	prettySteps := make(map[string]PrettyStatus)
	for k, v := range steps {
		prettySteps[k] = PrettyStatus{v}
	}
	prettySteps[substep.String()] = PrettyStatus{status}

	data, err := json.MarshalIndent(prettySteps, "", "  ") // pretty print JSON
	if err != nil {
		return err
	}

	// Use renameio to ensure atomicity when writing the status file.
	t, err := renameio.TempFile("", f.path)
	if err != nil {
		return err
	}
	defer func() {
		if cErr := t.Cleanup(); cErr != nil {
			err = multierror.Append(err, cErr).ErrorOrNil()
		}
	}()

	_, err = t.Write(data)
	if err != nil {
		return err
	}

	return t.CloseAtomicallyReplace()
}
