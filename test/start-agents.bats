#! /usr/bin/env bats

load helpers

setup() {
    kill_hub
}

@test "start-agents returns an error if the hub is not started" {
    run gpupgrade prepare start-agents
    [ "$status" -eq 1 ]
    [[ "$output" = *"couldn't connect to the upgrade hub (did you run 'gpupgrade prepare start-hub'?)"* ]]
}
