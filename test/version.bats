#! /usr/bin/env bats

# TODO: test also that we have a git version in the version string as well

@test "gpupgrade version prints version" {
    run gpupgrade version
    check_version
}

@test "gpupgrade --version prints version" {
    run gpupgrade -V
    check_version
}

@test "gpupgrade -V prints version" {
    run gpupgrade -V
    check_version
}

check_version() {
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" =~ ^gpupgrade[[:space:]]version[[:space:]][[:digit:]]\.[[:digit:]]\.[[:digit:]] ]]
}
