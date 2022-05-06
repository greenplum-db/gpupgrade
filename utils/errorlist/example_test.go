// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package errorlist_test

import (
	"errors"
	"fmt"

	"github.com/greenplum-db/gpupgrade/utils/errorlist"
)

func ExampleAppend() {
	errs := make(chan error)

	go func() {
		errs <- errors.New("one")
		errs <- errors.New("two")
		errs <- errors.New("three")
	}()

	var err error
	for i := 0; i < 3; i++ {
		err = errorlist.Append(err, <-errs)
	}

	fmt.Println(err)

	// Output:
	// 3 errors occurred:
	//	* one
	//	* two
	//	* three
}
