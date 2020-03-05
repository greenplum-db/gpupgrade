#! /usr/bin/env bats

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
        --source-bindir "$PWD" \
        --target-bindir "$PWD" \
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
    gpupgrade config set --target-bindir /my/new/bin/dir
    gpupgrade config set --source-bindir /my/old/bin/dir

    run gpupgrade config show --target-bindir
    echo $output
    [ "$status" -eq 0 ]
    [ "$output" = "/my/new/bin/dir" ]

    run gpupgrade config show --source-bindir
    [ "$status" -eq 0 ]
    [ "$output" = "/my/old/bin/dir" ]
}

@test "configuration persists after hub is killed and restarted" {
    gpupgrade config set --target-bindir /my/bin/dir

    gpupgrade kill-services
    gpupgrade hub --daemonize

    run gpupgrade config show --target-bindir
    [ "$status" -eq 0 ]
    [ "$output" = "/my/bin/dir" ]
}

@test "configuration can be dumped as a whole" {
    gpupgrade config set --target-bindir /my/new/bin/dir
    gpupgrade config set --source-bindir /my/old/bin/dir

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "source-bindir - /my/old/bin/dir" ]
    [ "${lines[1]}" = "target-bindir - /my/new/bin/dir" ]
    [ "${lines[2]}" = "target-datadir - " ] # This isn't populated until cluster creation, but it's still displayed here
}

@test "multiple configuration values can be set at once" {
    gpupgrade config set --target-bindir /my/new/bin/dir --source-bindir /my/old/bin/dir

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "source-bindir - /my/old/bin/dir" ]
    [ "${lines[1]}" = "target-bindir - /my/new/bin/dir" ]
}
