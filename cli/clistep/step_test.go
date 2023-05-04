// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package clistep_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/clistep"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/testutils"
	"github.com/greenplum-db/gpupgrade/utils"
)

func TestSubstep(t *testing.T) {
	t.Run("substep status is correctly printed on success and failure", func(t *testing.T) {
		stateDir, err := os.MkdirTemp("", "")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.RemoveAll(stateDir); err != nil {
				t.Errorf("removing temp directory: %v", err)
			}
		}()

		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
		defer resetEnv()

		d := BufferStandardDescriptors(t)

		st, err := clistep.Begin(idl.Step_initialize, false, true, "")
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		st.Run(idl.Substep_check_disk_space, func(streams step.OutStreams) error {
			return nil
		})

		err = errors.New("error")
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return err
		})

		err = st.Complete("")
		if err == nil {
			d.Close()
			t.Errorf("want err got nil")
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nInitialize in progress.\n\n"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_check_disk_space].OutputText, idl.Status_running) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_check_disk_space].OutputText, idl.Status_complete) + "\n"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_running) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_failed) + "\n"
		expected += "\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("there is no error when a hub substep is skipped", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		expected := step.Skip
		st.RunHubSubstep(func(streams step.OutStreams) error {
			return expected
		})

		err = st.Complete("")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if st.Err() != nil {
			t.Errorf("want err to be set to nil, got %#v", expected)
		}
	})

	t.Run("when a CLI substep is skipped its status is printed without error", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		skipErr := step.Skip
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return skipErr
		})

		err = st.Complete("")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if st.Err() != nil {
			t.Errorf("want err to be set to nil, got %#v", skipErr)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_running) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_skipped) + "\n\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("skips completed substeps", func(t *testing.T) {
		substepStore := &MockSubstepStore{Status: idl.Status_complete}
		st, err := clistep.NewStep(idl.Step_initialize, "initialize", &MockStepStore{}, substepStore, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		var called bool
		st.Run(idl.Substep_check_disk_space, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if called {
			t.Error("expected substep to be skipped")
		}
	})

	t.Run("AlwaysRun re-runs a completed substep", func(t *testing.T) {
		substepStore := &MockSubstepStore{Status: idl.Status_complete}
		st, err := clistep.NewStep(idl.Step_initialize, "initialize", &MockStepStore{}, substepStore, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		var called bool
		st.AlwaysRun(idl.Substep_check_disk_space, func(streams step.OutStreams) error {
			called = true
			return nil
		})

		if !called {
			t.Error("expected substep to be called")
		}
	})

	t.Run("errors when a substep was previously running", func(t *testing.T) {
		substepStore := &MockSubstepStore{Status: idl.Status_running}
		st, err := clistep.NewStep(idl.Step_initialize, "initialize", &MockStepStore{}, substepStore, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		st.Run(idl.Substep_check_disk_space, func(streams step.OutStreams) error {
			return nil
		})

		err = st.Complete("")
		expected := fmt.Sprintf("Found previous substep %s was running. Manual intervention needed to cleanup.", idl.Substep_check_disk_space)
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("expected err %#v to contain %q", err, expected)
		}
	})

	t.Run("when a CLI substep is quit by the user its status is printed without the generic next action error", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		quitErr := step.Quit
		st.Run(idl.Substep_generate_data_migration_scripts, func(streams step.OutStreams) error {
			return quitErr
		})

		err = st.Complete("")
		if !errors.Is(err, step.Quit) {
			t.Errorf("unexpected err %#v", err)
		}

		if !errors.Is(st.Err(), step.Quit) {
			t.Errorf("expected err to be quit, got %#v", quitErr)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := commanders.Format(commanders.SubstepDescriptions[idl.Substep_generate_data_migration_scripts].OutputText, idl.Status_running) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_generate_data_migration_scripts].OutputText, idl.Status_quit) + "\n\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("substeps are not run when a hub substep errors", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		err = errors.New("error")
		st.RunHubSubstep(func(streams step.OutStreams) error {
			return err
		})

		ran := false
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			ran = true
			return nil
		})

		st.RunHubSubstep(func(streams step.OutStreams) error {
			ran = true
			return nil
		})

		err = st.Complete("")
		if err == nil {
			t.Errorf("expected error")
		}

		if ran {
			t.Error("expected substep to not be run")
		}

		if st.Err() == nil {
			t.Error("expected error")
		}
	})

	t.Run("cli substeps are printed to stdout and stderr in verbose mode", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, true)
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		substepStdout := "some substep output text."
		substepStderr := "oops!"
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			os.Stdout.WriteString(substepStdout)
			os.Stderr.WriteString(substepStderr)
			return nil
		})

		err = st.Complete("")
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		expectedStdout := commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_running)
		expectedStdout += substepStdout + "\n\r"
		expectedStdout += commanders.Format(commanders.SubstepDescriptions[idl.Substep_saving_source_cluster_config].OutputText, idl.Status_complete) + "\n"
		expectedStdout += fmt.Sprintf("%s took", idl.Substep_saving_source_cluster_config)

		stdout, stderr := d.Collect()
		d.Close()
		actualStdout := string(stdout)
		// Use HasPrefix since we don't know the actualStdout step duration.
		if !strings.HasPrefix(actualStdout, expectedStdout) {
			t.Errorf("stdout %#v want %#v", actualStdout, expectedStdout)
			t.Logf("actualStdout: %s", actualStdout)
			t.Logf("expectedStdout: %s", expectedStdout)
		}

		actualStderr := string(stderr)
		if actualStderr != substepStderr {
			t.Errorf("stderr %#v want %#v", actualStdout, expectedStdout)
		}
	})

	t.Run("cli substeps are not run when there is an error", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		err = errors.New("error")
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return err
		})

		ran := false
		st.Run(idl.Substep_start_hub, func(streams step.OutStreams) error {
			ran = true
			return nil
		})

		err = st.Complete("")
		if err == nil {
			t.Errorf("expected error")
		}

		if ran {
			t.Error("expected cli substep to not be run")
		}

		if st.Err() == nil {
			t.Error("expected error")
		}
	})

	t.Run("hub substeps are not run when there is an error", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		err = errors.New("error")
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return err
		})

		ran := false
		st.RunHubSubstep(func(streams step.OutStreams) error {
			ran = true
			return nil
		})

		err = st.Complete("")
		if err == nil {
			t.Errorf("expected error")
		}

		if ran {
			t.Error("expected hub substep to not be run")
		}

		if st.Err() == nil {
			t.Error("expected error")
		}
	})

	t.Run("fails to create a new step when the state directory does not exist", func(t *testing.T) {
		resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", "/does/not/exist")
		defer resetEnv()

		_, err := clistep.Begin(idl.Step_initialize, false, true, "")
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got %T, want %T", err, nextActionsErr)
		}

		if nextActionsErr.NextAction != clistep.RunInitialize {
			t.Errorf("got %q want %q", nextActionsErr.NextAction, clistep.RunInitialize)
		}
	})

	t.Run("substeps can override the default next actions error", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		nextAction := "re-run gpupgrade"
		st.RunHubSubstep(func(streams step.OutStreams) error {
			return utils.NewNextActionErr(errors.New("oops"), nextAction)
		})

		err = st.Complete("")
		var nextActions utils.NextActionErr
		if !errors.As(err, &nextActions) {
			t.Errorf("got type %T want %T", err, nextActions)
		}

		genericNextAction := "Please address the above issue and run \"gpupgrade initialize\" again.\nIf you would like to return the cluster to its original state, please run \"gpupgrade revert\".\n"
		expected := nextAction + "\n\n" + genericNextAction
		if nextActions.NextAction != expected {
			t.Errorf("got next action %q want %q", nextActions.NextAction, expected)
		}
	})

	t.Run("substep duration is printed", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, true)
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return nil
		})

		err = st.Complete("")
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		actual := string(stdout)
		expected := fmt.Sprintf("\n%s took", idl.Substep_saving_source_cluster_config)
		// Use Contains since we don't know the actual step duration.
		if !strings.Contains(actual, expected) {
			t.Errorf("expected output %#v to end with %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("the step returns next actions when a substep fails", func(t *testing.T) {
		st, err := clistep.NewStep(idl.Step_initialize, idl.Step_initialize.String(), &MockStepStore{}, &MockSubstepStore{}, &step.BufferedStreams{}, false)
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		expected := errors.New("oops")
		st.Run(idl.Substep_saving_source_cluster_config, func(streams step.OutStreams) error {
			return expected
		})

		err = st.Complete("")
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got %T, want %T", err, nextActionsErr)
		}
	})
}

func TestStepStatus(t *testing.T) {
	stateDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			t.Errorf("removing temp directory: %v", err)
		}
	}()

	resetEnv := testutils.SetEnv(t, "GPUPGRADE_HOME", stateDir)
	defer resetEnv()

	stepStore, err := clistep.NewStepFileStore()
	if err != nil {
		t.Fatalf("NewStepStore failed: %v", err)
	}

	t.Run("when a step is created its status is set to running", func(t *testing.T) {
		_, err := clistep.Begin(idl.Step_initialize, false, true, "")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		status, err := stepStore.Read(idl.Step_initialize)
		if err != nil {
			t.Errorf("Read failed %#v", err)
		}

		expected := idl.Status_running
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("when the step store is disabled step.Complete does not update the status", func(t *testing.T) {
		st, err := clistep.Begin(idl.Step_initialize, false, true, "")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		st.DisableStore()

		err = st.Complete("")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		status, err := stepStore.Read(idl.Step_initialize)
		if err != nil {
			t.Errorf("Read failed %#v", err)
		}

		expected := idl.Status_running
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("when a hub substep fails it sets the step status to failed", func(t *testing.T) {
		st, err := clistep.Begin(idl.Step_initialize, false, true, "")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		st.RunHubSubstep(func(streams step.OutStreams) error {
			return errors.New("oops")
		})

		err = st.Complete("")
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got %T, want %T", err, nextActionsErr)
		}

		status, err := stepStore.Read(idl.Step_initialize)
		if err != nil {
			t.Errorf("Read failed %#v", err)
		}

		expected := idl.Status_failed
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("when a cli substep fails it sets the step status to failed", func(t *testing.T) {
		st, err := clistep.Begin(idl.Step_initialize, false, true, "")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		st.Run(idl.Substep_check_disk_space, func(streams step.OutStreams) error {
			return errors.New("oops")
		})

		err = st.Complete("")
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got %T, want %T", err, nextActionsErr)
		}

		status, err := stepStore.Read(idl.Step_initialize)
		if err != nil {
			t.Errorf("Read failed %#v", err)
		}

		expected := idl.Status_failed
		if status != expected {
			t.Errorf("got stauts %q want %q", status, expected)
		}
	})

	t.Run("confirmation text is not printed when a step is invalid", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		_, err := clistep.Begin(idl.Step_execute, false, true, "confirmation text")
		var nextActionsErr utils.NextActionErr
		if !errors.As(err, &nextActionsErr) {
			d.Close()
			t.Errorf("got %T want %T", err, nextActionsErr)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		if len(stdout) != 0 {
			t.Errorf("unexpected stdout %#v", string(stdout))
		}
	})

	t.Run("confirmation text is printed when not in non-interactive mode", func(t *testing.T) {
		dir := testutils.GetTempDir(t, "")
		defer testutils.MustRemoveAll(t, dir)

		stdinFile := filepath.Join(dir, "stdin.txt")
		testutils.MustWriteToFile(t, stdinFile, "y\n")

		stdin, err := os.Open(stdinFile)
		if err != nil {
			t.Errorf("opening %q: %v", stdinFile, err)
		}

		oldStdin := os.Stdin
		os.Stdin = stdin
		defer func() { os.Stdin = oldStdin }()

		d := BufferStandardDescriptors(t)

		_, err = clistep.Begin(idl.Step_initialize, false, false, "confirmation text")
		if err != nil {
			t.Errorf("NewStep returned error: %#v", err)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := `confirmation text
Continue with gpupgrade initialize?  Yy|Nn: 
Proceeding with upgrade

Initialize in progress.

`
		actual := string(stdout)
		if actual != expected {
			t.Errorf("got output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("confirmation text is not printed in non-interactive mode", func(t *testing.T) {
		d := BufferStandardDescriptors(t)

		_, err := clistep.Begin(idl.Step_initialize, false, true, "confirmation text")
		if err != nil {
			t.Errorf("NewStep returned error: %#v", err)
		}

		stdout, stderr := d.Collect()
		d.Close()
		if len(stderr) != 0 {
			t.Errorf("unexpected stderr %#v", string(stderr))
		}

		expected := "\nInitialize in progress.\n\n"
		actual := string(stdout)
		if actual != expected {
			t.Errorf("got output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})
}

func TestPrompt(t *testing.T) {
	t.Run("returns error when failing to read input", func(t *testing.T) {
		input := ""
		reader := bufio.NewReader(strings.NewReader(input))
		err := clistep.Prompt(reader, idl.Step_execute)
		if err != io.EOF {
			t.Errorf("Prompt(%q) returned error: %+v ", input, io.EOF)
		}
	})

	t.Run("returns true when user proceeds", func(t *testing.T) {
		for _, input := range []string{"y\n", "Y\n"} {
			reader := bufio.NewReader(strings.NewReader(input))
			err := clistep.Prompt(reader, idl.Step_execute)
			if err != nil {
				t.Errorf("Prompt(%q) returned error: %+v ", input, err)
			}
		}
	})

	t.Run("returns step.Quit when user cancels", func(t *testing.T) {
		for _, input := range []string{"n\n", "N\n"} {
			reader := bufio.NewReader(strings.NewReader(input))
			err := clistep.Prompt(reader, idl.Step_execute)
			if !errors.Is(err, step.Quit) {
				t.Errorf("unexpected error %#v", err)
			}
		}
	})
}

type MockStepStore struct {
	Status   idl.Status
	WriteErr error
}

func (t *MockStepStore) Read(_ idl.Step) (idl.Status, error) {
	return t.Status, nil
}

func (t *MockStepStore) Write(_ idl.Step, status idl.Status) error {
	t.Status = status
	return t.WriteErr
}

func (s *MockStepStore) HasStepStarted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status != idl.Status_unknown_status
	})
}

func (s *MockStepStore) HasStepNotStarted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status == idl.Status_unknown_status
	})
}

func (s *MockStepStore) HasStepCompleted(step idl.Step) (bool, error) {
	return s.HasStatus(step, func(status idl.Status) bool {
		return status == idl.Status_complete
	})
}

func (s *MockStepStore) HasStatus(step idl.Step, check func(status idl.Status) bool) (bool, error) {
	status, err := s.Read(step)
	if err != nil {
		return false, err
	}

	return check(status), nil
}

type MockSubstepStore struct {
	Status   idl.Status
	WriteErr error
}

func (t *MockSubstepStore) Read(_ idl.Step, substep idl.Substep) (idl.Status, error) {
	return t.Status, nil
}

func (t *MockSubstepStore) Write(_ idl.Step, substep idl.Substep, status idl.Status) error {
	t.Status = status
	return t.WriteErr
}
