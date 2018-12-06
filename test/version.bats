#! /usr/bin/env bats

# TODO: test also that we have a git version in the version string as well 

@test "gpupgrade version subcommand reasonable" {
    run gpupgrade version
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" =~ ^gpupgrade[[:space:]]version[[:space:]][[:digit:]]\.[[:digit:]]\.[[:digit:]] ]]
}

@test "gpupgrade_hub version subcommand reasonable" {
    run gpupgrade_hub --version
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" =~ ^gpupgrade_hub[[:space:]]version[[:space:]][[:digit:]]\.[[:digit:]]\.[[:digit:]] ]]
}

@test "gpupgrade_agent version subcommand reasonable" {
    run gpupgrade_agent --version
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" =~ ^gpupgrade_agent[[:space:]]version[[:space:]][[:digit:]]\.[[:digit:]]\.[[:digit:]] ]]
}