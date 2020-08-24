//  Copyright (c) 2017-2020 VMware, Inc. or its affiliates
//  SPDX-License-Identifier: Apache-2.0

package commanders_test

import (
	"errors"
	"testing"

	"github.com/greenplum-db/gpupgrade/cli/commanders"
	"github.com/greenplum-db/gpupgrade/idl"
)

func TestSubstep(t *testing.T) {
	d := commanders.BufferStandardDescriptors(t)
	defer d.Close()

	var err error
	s := commanders.NewSubstep(idl.Substep_CREATING_DIRECTORIES, false)
	s.Finish(&err)

	err = errors.New("error")
	s = commanders.NewSubstep(idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG, false)
	s.Finish(&err)

	stdout, stderr := d.Collect()

	if len(stderr) != 0 {
		t.Errorf("unexpected stderr %#v", string(stderr))
	}

	expected := commanders.Format(commanders.SubstepDescriptions[idl.Substep_CREATING_DIRECTORIES].OutputText, idl.Status_RUNNING) + "\r"
	expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_CREATING_DIRECTORIES].OutputText, idl.Status_COMPLETE) + "\n"
	expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_RUNNING) + "\r"
	expected += commanders.Format(commanders.SubstepDescriptions[idl.Substep_SAVING_SOURCE_CLUSTER_CONFIG].OutputText, idl.Status_FAILED) + "\n"

	actual := string(stdout)
	if actual != expected {
		t.Errorf("output %#v want %#v", actual, expected)
	}
}
