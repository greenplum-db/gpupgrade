#! /usr/bin/env bats

load helpers

setup() {
    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    kill_agents
    kill_hub
    gpupgrade initialize --old-bindir "${GPHOME}/bin" --new-bindir "${GPHOME}/bin" --old-port ${PGPORT}
}

teardown() {
    kill_agents
    kill_hub
    rm -r "${STATE_DIR}"
}

@test "configuration can be read after it is written" {
    gpupgrade config set --new-bindir /my/new/bin/dir
    gpupgrade config set --old-bindir /my/old/bin/dir

    run gpupgrade config show --new-bindir
    echo $output
    [ "$status" -eq 0 ]
    [ "$output" = "/my/new/bin/dir" ]

    run gpupgrade config show --old-bindir
    [ "$status" -eq 0 ]
    [ "$output" = "/my/old/bin/dir" ]
}

# TODO: we need to rewrite this test after we work through the design
#   of how `gpupgrade recover` will work
#
@test "configuration persists after hub is killed and restarted" {
   gpupgrade config set --new-bindir /my/bin/dir
   kill_hub

   gpupgrade_hub --daemonize

   run gpupgrade config show --new-bindir
   [ "$status" -eq 0 ]
   [ "$output" = "/my/bin/dir" ]
}

@test "configuration can be dumped as a whole" {
    gpupgrade config set --new-bindir /my/new/bin/dir
    gpupgrade config set --old-bindir /my/old/bin/dir

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "new-bindir - /my/new/bin/dir" ]
    [ "${lines[1]}" = "old-bindir - /my/old/bin/dir" ]
}

@test "multiple configuration values can be set at once" {
    gpupgrade config set --new-bindir /my/new/bin/dir --old-bindir /my/old/bin/dir

    run gpupgrade config show
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "new-bindir - /my/new/bin/dir" ]
    [ "${lines[1]}" = "old-bindir - /my/old/bin/dir" ]
}
