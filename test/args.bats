#! /usr/bin/env bats

load helpers

@test "gpupgrade outputs help text when no params are given" {
    run gpupgrade

    [ "$status" -eq 1 ]
    [[ "$output" = *"Please specify one command of: check, config, initialize, prepare, status, upgrade, or version"* ]]
}

@test "gpupgrade subcommands fail when passed insufficient arguments" {
    run gpupgrade initialize
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'Required flag(s) "new-bindir", "old-bindir", "old-port" have/has not been set'* ]]; then
        fail "actual: $output"
    fi

    run gpupgrade config set
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'the set command requires at least one flag to be specified'* ]]; then
        fail "actual: $output"
    fi
}
