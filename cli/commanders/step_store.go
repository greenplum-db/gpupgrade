//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

// StepStore tracks the overall step status such as running, failed, or completed
// for initialize, execute, finalize, and revert. To reduce the code change
// required and for convenience StepStore uses the same data structure and
// file store as substeps.json. An internal substep enum STEP_STATUS is used to
// track the overall step status and should not be used as a normal substep.
type StepStore struct {
	store *step.FileStore
}

func NewStepStore() (*StepStore, error) {
	path, err := utils.GetJSONFile(utils.GetStateDir(), StepsFileName)
	if err != nil {
		return &StepStore{}, xerrors.Errorf("getting %q file: %w", StepsFileName, err)
	}

	return &StepStore{
		store: step.NewFileStore(path),
	}, nil
}

func (s *StepStore) Write(stepName idl.Step, status idl.Status) error {
	err := s.store.Write(stepName, idl.Substep_STEP_STATUS, status)
	if err != nil {
		return err
	}

	return nil
}

func (s *StepStore) Read(stepName idl.Step) (idl.Status, error) {
	status, err := s.store.Read(stepName, idl.Substep_STEP_STATUS)
	if err != nil {
		return idl.Status_UNKNOWN_STATUS, err
	}

	return status, nil
}
