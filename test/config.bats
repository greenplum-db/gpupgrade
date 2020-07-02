#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    # XXX We use $PWD here instead of a real binary directory because
    # `make check` is expected to test the locally built binaries, not the
    # installation. This causes problems for tests that need to call GPDB
    # executables...
    gpupgrade initialize \
        --source-gphome "$PWD" \
        --target-gphome "$PWD" \
        --source-master-port ${PGPORT} \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -z "${BATS_TEST_SKIPPED}" ]; then
        gpupgrade kill-services
        rm -r "${STATE_DIR}"
    fi
}

@test "configuration can be read after it is written" {
    gpupgrade config set --target-gphome /usr/local/target
    gpupgrade config set --source-gphome /usr/local/source

    run gpupgrade config show --target-gphome
    echo $output
    [ "$status" -eq 0 ]
    [ "$output" = "/usr/local/target" ]

    run gpupgrade config show --source-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "/usr/local/source" ]
}

@test "configuration persists after hub is killed and restarted" {
    gpupgrade config set --target-gphome /usr/local/target

    gpupgrade kill-services
    gpupgrade hub --daemonize

    run gpupgrade config show --target-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "/usr/local/target" ]
}

@test "configuration can be dumped as a whole" {
    gpupgrade config set --target-gphome /usr/local/target
    gpupgrade config set --source-gphome /usr/local/source

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" = "id - "* ]] # this is randomly generated; we could replace * with a base64 regex matcher
    [ "${lines[1]}" = "source-gphome - /usr/local/source" ]
    [ "${lines[2]}" = "target-datadir - " ] # This isn't populated until cluster creation, but it's still displayed here
    [ "${lines[3]}" = "target-gphome - /usr/local/target" ]
}

@test "multiple configuration values can be set at once" {
    gpupgrade config set --target-gphome /usr/local/target --source-gphome /usr/local/source

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [ "${lines[1]}" = "source-gphome - /usr/local/source" ]
    [ "${lines[3]}" = "target-gphome - /usr/local/target" ]
}
