#! /usr/bin/env bats
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

@test "gpupupgrade is compiled with golang version 1.15.X" {
    local EXPECTED="gpupgrade: go1.15."
    run go version gpupgrade
    [ "$status" -eq 0 ]
    [[ "$output" =~ $EXPECTED ]] || fail "expected: $EXPECTED got: $output"
}
