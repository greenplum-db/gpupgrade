// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package commanders

import (
	"errors"

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
type StepStore interface {
	Read(idl.Step) (idl.Status, error)
	Write(idl.Step, idl.Status) error
	HasStepStarted(idl.Step) (bool, error)
	HasStepNotStarted(idl.Step) (bool, error)
	HasStepCompleted(idl.Step) (bool, error)
	HasStatus(idl.Step, func(status idl.Status) bool) (bool, error)
}

type StepStoreFileStore struct {
	store *step.SubstepFileStore
}

func NewStepFileStore() (*StepStoreFileStore, error) {
	path, err := utils.GetJSONFile(utils.GetStateDir(), StepsFileName)
	if err != nil {
		return &StepStoreFileStore{}, xerrors.Errorf("getting %q file: %w", StepsFileName, err)
	}

	return &StepStoreFileStore{store: step.NewSubstepStoreUsingFile(path)}, nil
}

func (s *StepStoreFileStore) Write(stepName idl.Step, status idl.Status) error {
	err := s.store.Write(stepName, idl.Substep_step_status, status)
	if err != nil {
		return err
	}

	return nil
}

func (s *StepStoreFileStore) Read(stepName idl.Step) (idl.Status, error) {
	status, err := s.store.Read(stepName, idl.Substep_step_status)
	if err != nil {
		return idl.Status_unknown_status, err
	}

	return status, nil
}

func (s *StepStoreFileStore) HasStepStarted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status != idl.Status_unknown_status
	})
}

func (s *StepStoreFileStore) HasStepNotStarted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status == idl.Status_unknown_status
	})
}

func (s *StepStoreFileStore) HasStepCompleted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status == idl.Status_complete
	})
}

func (s *StepStoreFileStore) HasStatus(step idl.Step, check func(status idl.Status) bool) (bool, error) {
	status, err := s.Read(step)
	if err != nil {
		return false, err
	}

	return check(status), nil
}

type stepCondition struct {
	idl.Step
	condition  func(s *StepStoreFileStore, step idl.Step) (bool, error)
	nextAction string
}

var StepErr = errors.New(`gpupgrade commands must be issued in correct order

  1. initialize   runs pre-upgrade checks and prepares the cluster for upgrade
  2. execute      upgrades the master and primary segments to the target
                  Greenplum version
  3. finalize     upgrades the standby master and mirror segments to the target
                  Greenplum version. Revert cannot be run after finalize has started.

Use "gpupgrade --help" for more information`)

const RunInitialize = `To begin the upgrade, run "gpupgrade initialize".`

const RunExecute = `To proceed with the upgrade, run "gpupgrade execute".
To return the cluster to its original state, run "gpupgrade revert".`

const RunFinalize = `To proceed with the upgrade, run "gpupgrade finalize".`

const RunRevert = `Revert is in progress. Please continue by running "gpupgrade revert".`

// conditions expected to have been met for the current step. The next action
// message is printed if the condition is not met.
var validate = map[idl.Step][]stepCondition{
	idl.Step_initialize: {
		{idl.Step_revert, (*StepStoreFileStore).HasStepNotStarted, RunRevert},
		{idl.Step_finalize, (*StepStoreFileStore).HasStepNotStarted, RunFinalize},
		{idl.Step_execute, (*StepStoreFileStore).HasStepNotStarted, RunExecute},
	},
	idl.Step_execute: {
		{idl.Step_revert, (*StepStoreFileStore).HasStepNotStarted, RunRevert},
		{idl.Step_initialize, (*StepStoreFileStore).HasStepCompleted, RunInitialize},
		{idl.Step_finalize, (*StepStoreFileStore).HasStepNotStarted, RunFinalize},
	},
	idl.Step_finalize: {
		{idl.Step_revert, (*StepStoreFileStore).HasStepNotStarted, RunRevert},
		{idl.Step_initialize, (*StepStoreFileStore).HasStepCompleted, RunInitialize},
		{idl.Step_execute, (*StepStoreFileStore).HasStepCompleted, RunExecute},
	},
	idl.Step_revert: {
		{idl.Step_initialize, (*StepStoreFileStore).HasStepStarted, RunInitialize},
		{idl.Step_finalize, (*StepStoreFileStore).HasStepNotStarted, RunFinalize},
	},
}

func (s *StepStoreFileStore) ValidateStep(currentStep idl.Step) (err error) {
	conditions := validate[currentStep]
	for _, c := range conditions {
		status, err := c.condition(s, c.Step)
		if err != nil {
			return err
		}

		if !status {
			return utils.NewNextActionErr(StepErr, c.nextAction)
		}
	}

	return nil
}
