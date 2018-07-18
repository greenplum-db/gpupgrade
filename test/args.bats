#! /usr/bin/env bats

@test "gpupgrade outputs help text when no params are given" {
    run gpupgrade

    [ "$status" -eq 1 ]
    [[ "$output" = *"Please specify one command of: check, prepare, status, upgrade, or version"* ]]
}

# XXX The amount of copy-paste here is ugly, as is the fact that the test stops
# at the first failure -- but I expect most of this to go away when we remove
# the requirement to pass every option to every step, and then we can refactor
# whatever remains.
@test "gpupgrade subcommands fail when passed insufficient arguments" {
    run gpupgrade check config
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "master-host", "old-bindir" have/has not been set'* ]]

    run gpupgrade check disk-space
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "master-host" have/has not been set'* ]]

    run gpupgrade check object-count
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "master-host" have/has not been set'* ]]

    run gpupgrade check seginstall
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "master-host" have/has not been set'* ]]

    run gpupgrade check version
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "master-host" have/has not been set'* ]]

    run gpupgrade prepare init-cluster
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "new-bindir, "port" have/has not been set'* ]]

    run gpupgrade prepare shutdown-clusters
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "new-bindir", "old-bindir" have/has not been set'* ]]

    run gpupgrade upgrade convert-master
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "new-bindir", "new-datadir", "old-bindir", "old-datadir" have/has not been set'* ]]

    run gpupgrade upgrade convert-primaries
    [ "$status" -eq 1 ]
    [[ "$output" = *'Required flag(s) "new-bindir", "old-bindir" have/has not been set'* ]]
}
