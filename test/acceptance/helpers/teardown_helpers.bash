# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

#
# Helpers for performing more complex teardowns. Register a command to run with
# register_teardown, and run all teardowns in reverse order of registration with
# run_teardowns.
#

TEARDOWN_FUNCTIONS=()

# Registers a command to be run during run_teardowns. Commands may have
# arbitrary parameters; just keep in mind that Bash evaluates them during
# register_teardown, not run_teardowns.
register_teardown() {
    # Bash doesn't have arrays of arrays. Fake them by length-prefixing each new
    # "row" in the list. NOTE: there is probably a better way to do this in Bash
    # 4, but many of us are running Bash 3 on macOS.
    local len=${#@}
    TEARDOWN_FUNCTIONS=( "$len" "$@" "${TEARDOWN_FUNCTIONS[@]}")
}

# Runs all teardown commands (registered via register_teardown) in LIFO order
# and clears the list of teardowns.
run_teardowns() {
    _run "${TEARDOWN_FUNCTIONS[@]}" || return $?

    TEARDOWN_FUNCTIONS=()
}

# Internal implementation for run_teardowns.
_run() {
    while (( ${#@} )); do
        # Pop the length of the next command.
        local len=$1; shift
        local cmd=()

        # Pop each element of the command into our list.
        for (( i = 0; i < len; i++ )); do
            cmd+=( "$1" ); shift
        done

        # Echo the teardown function, properly shell-quoted, to help debugging.
        printf 'running teardown:'
        printf ' %q' "${cmd[@]}"
        printf '\n'

        # Run.
        "${cmd[@]}" || return $?
    done
}
