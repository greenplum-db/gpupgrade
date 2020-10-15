//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli"
	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
)

func TestSubstep(t *testing.T) {
	t.Run("substep status is correctly printed on success and failure", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		st.RunCLISubstep(idl.Substep_CHECK_DISK_SPACE, func(streams step.OutStreams) error {
			return nil
		})

		err := errors.New("error")
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_CHECK_DISK_SPACE].OutputText, idl.Status_RUNNING) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_CHECK_DISK_SPACE].OutputText, idl.Status_COMPLETE) + "\n"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_RUNNING) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_FAILED) + "\n"
		expected += "\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("there is no error when a hub substep is skipped", func(t *testing.T) {
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		expected := step.Skip
		st.RunHubSubstep(func(streams step.OutStreams) error {
			return expected
		})

		err := st.Complete("")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if st.Err() != nil {
			t.Errorf("want err to be set to nil, got %#v", expected)
		}
	})

	t.Run("when a CLI substep is skipped its status is printed without error", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		skipErr := step.Skip
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			return skipErr
		})

		err := st.Complete("")
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

		expected := "\nInitialize in progress.\n\n"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_RUNNING) + "\r"
		expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_SKIPPED) + "\n"
		expected += "\n"

		actual := string(stdout)
		if actual != expected {
			t.Errorf("output %#v want %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("there is no error when an internal substep is skipped", func(t *testing.T) {
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		skipErr := step.Skip
		st.RunInternalSubstep(func() error {
			return skipErr
		})

		err := st.Complete("")
		if err != nil {
			t.Errorf("unexpected err %#v", err)
		}

		if st.Err() != nil {
			t.Errorf("want err to be set to nil, got %#v", skipErr)
		}
	})

	t.Run("both cli and hub substeps are not run when an internal substep errors", func(t *testing.T) {
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		err := errors.New("error")
		st.RunInternalSubstep(func() error {
			return err
		})

		ran := false
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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

	t.Run("nothing is printed for internal substeps", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, true)

		ran := false
		st.RunInternalSubstep(func() error {
			ran = true
			return nil
		})

		err := st.Complete("")
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		if !ran {
			d.Close()
			t.Error("expected hub substep to be run")
		}

		if st.Err() != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		expectedStdout := "\nInitialize in progress.\n\n"
		expectedStdout += "Initialize took 0s\n\n"
		expectedStdout += "\n"

		stdout, stderr := d.Collect()
		d.Close()
		actualStdout := string(stdout)
		if actualStdout != expectedStdout {
			t.Errorf("stdout %#v want %#v", actualStdout, expectedStdout)
			t.Logf("actualStdout: %s", actualStdout)
			t.Logf("expectedStdout: %s", expectedStdout)
		}

		actualStderr := string(stderr)
		expectedStderr := ""
		if actualStderr != expectedStderr {
			t.Errorf("stderr %#v want %#v", actualStdout, expectedStderr)
		}
	})

	t.Run("cli substeps are printed to stdout and stderr in verbose mode", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, true)

		substepStdout := "some substep output text."
		substepStderr := "oops!"
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			os.Stdout.WriteString(substepStdout)
			os.Stderr.WriteString(substepStderr)
			return nil
		})

		err := st.Complete("")
		if err != nil {
			d.Close()
			t.Errorf("unexpected err %#v", err)
		}

		expectedStdout := "\nInitialize in progress.\n\n"
		expectedStdout += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_RUNNING)
		expectedStdout += substepStdout + "\n\r"
		expectedStdout += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_COMPLETE) + "\n"
		expectedStdout += fmt.Sprintf("%s took", idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)

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
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		err := errors.New("error")
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			return err
		})

		ran := false
		st.RunCLISubstep(idl.Substep_START_HUB, func(streams step.OutStreams) error {
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
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		err := errors.New("error")
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
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

	t.Run("substeps can override the default next actions error", func(t *testing.T) {
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		subcommand := "subcommand"
		st.RunHubSubstep(func(streams step.OutStreams) error {
			return cli.NewNextActions(errors.New("oops"), subcommand, false)
		})

		err := st.Complete("")
		var nextActions cli.NextActions
		if !errors.As(err, &nextActions) {
			t.Errorf("got type %T want %T", err, nextActions)
		}

		if nextActions.Subcommand != subcommand {
			t.Errorf("got subcommand %q want %q", nextActions.Subcommand, subcommand)
		}
	})

	t.Run("substep duration is printed", func(t *testing.T) {
		d := commanders.BufferStandardDescriptors(t)

		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, true)

		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			return nil
		})

		err := st.Complete("")
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
		expected := fmt.Sprintf("\n%s took", idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG)
		// Use Contains since we don't know the actual step duration.
		if !strings.Contains(actual, expected) {
			t.Errorf("expected output %#v to end with %#v", actual, expected)
			t.Logf("actual: %s", actual)
			t.Logf("expected: %s", expected)
		}
	})

	t.Run("the step returns next actions when a substep fails", func(t *testing.T) {
		st := commanders.NewStep(idl.Step_INITIALIZE, &step.BufferedStreams{}, false)

		expected := errors.New("oops")
		st.RunCLISubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, func(streams step.OutStreams) error {
			return expected
		})

		err := st.Complete("")
		var nextActionsErr cli.NextActions
		if !errors.As(err, &nextActionsErr) {
			t.Errorf("got %T, want %T", err, nextActionsErr)
		}
	})
}
