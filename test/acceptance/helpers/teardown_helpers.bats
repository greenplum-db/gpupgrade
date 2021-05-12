#! /usr/bin/env bats
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers
load ../helpers/teardown_helpers

# Prints the number of arguments passed, and the arguments themselves, on
# separate lines.
print_args() {
    echo ${#@}
    for arg in "$@"; do
        echo "$arg"
    done
}

@test "run_teardowns runs all registered commands in reverse order" {
    register_teardown echo first
    register_teardown print_args one "two three" four
    register_teardown echo last
    
    run run_teardowns

    diff -U2 <(echo "$output") - <<'EOF' || fail "run_teardowns printed unexpected output (see diff)"
running teardown: echo last
last
running teardown: print_args one two\ three four
3
one
two three
four
running teardown: echo first
first
EOF
}

@test "run_teardowns clears the registered list" {
    local was_run=0
    register_teardown eval was_run=1

    run_teardowns
    (( was_run )) || fail "teardown was not run"

    was_run=0
    run_teardowns
    (( ! was_run )) || fail "teardown was re-run incorrectly"
}

@test "teardown failures are reported even without set -e" {
    local status=0

    register_teardown false

    # Chaining commands with || will disable set -e.
    run_teardowns || status=$?
    [ "$status" -eq 1 ] || fail "status was $status; expected 1"
}

@test "teardown failures fail fast even without set -e" {
    local was_run=0

    register_teardown eval was_run=1  # runs last
    register_teardown false           # runs first

    run_teardowns || true # disable set -e

    (( ! was_run )) || fail "second teardown was run unexpectedly"
}
