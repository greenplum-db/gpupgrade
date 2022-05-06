#! /usr/bin/env bats
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers

@test "gpupgrade is compiled with the expected golang version from go.mod" {
    pushd "${BATS_TEST_DIRNAME}/../../../"
        local version expected
        version=$(sed -n -E 's/^go ([0-9].[0-9]+)/\1/p' go.mod)
        expected="gpupgrade: go${version}"

        run go version gpupgrade
        [ "$status" -eq 0 ]
        [[ "$output" =~ $expected ]] || fail "expected: $expected got: $output"
    popd
}
