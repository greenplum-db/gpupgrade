#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

@test "gpupgrade subcommands fail when passed insufficient arguments" {
    run gpupgrade initialize
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'Required flag(s) "source-bindir", "source-master-port", "target-bindir" have/has not been set'* ]]; then
        fail "actual: $output"
    fi

    run gpupgrade config set
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'the set command requires at least one flag to be specified'* ]]; then
        fail "actual: $output"
    fi
}
