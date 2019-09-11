#! /usr/bin/env bats

load helpers

setup() {
    require_gpdb
    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    kill_agents
    kill_hub
    gpupgrade initialize --old-bindir="${GPHOME}/bin" --new-bindir="${GPHOME}/bin" --old-port="${PGPORT}"
    kill_hub
}

teardown() {
    kill_agents
    kill_hub
}

@test "start-hub fails if the source configuration hasn't been initialized" {
	rm $GPUPGRADE_HOME/source_cluster_config.json
    run gpupgrade_hub --daemonize
    [ "$status" -eq 1 ]

    [[ "$output" = *"Unable to load source cluster configuration"* ]]
}

@test "start-hub fails if the target configuration hasn't been initialized" {
	rm $GPUPGRADE_HOME/target_cluster_config.json
    run gpupgrade_hub --daemonize
    [ "$status" -eq 1 ]

    [[ "$output" = *"Unable to load target cluster configuration"* ]]
}

@test "initialize returns an error when it is ran twice" {
    # second start should return an error
    ! gpupgrade initialize --old-bindir="${GPHOME}/bin" --new-bindir="${GPHOME}/bin" --old-port="${PGPORT}"
    # TODO: check for a useful error message
}

@test "initialize does not return an error if an unrelated process has gpupgrade_hub in its name" {
    # Create a long-running process with gpupgrade_hub in the name.
    exec -a gpupgrade_hub_test_log sleep 5 3>&- &
    bgproc=$! # save the PID to kill later

    # Wait a little bit for the background process to get its new name.
    while ! ps -ef | grep -Gq "[g]pupgrade_hub"; do
        sleep .001

        # To avoid hanging forever if something goes terribly wrong, make sure
        # the background process still exists during every iteration.
        kill -0 $bgproc
    done

    # Start the hub; there should be no errors.
    gpupgrade_hub --daemonize 3>&-

    # Clean up. Use SIGINT rather than SIGTERM to avoid a nasty-gram from BATS.
    kill -INT $bgproc
}

outputContains() {
    [[ "$output" = *"$1"* ]]
}

@test "subcommands return an error if the hub is not started" {
    commands=(
        'prepare shutdown-clusters'
        'prepare init-cluster'
        'config set --old-bindir /dummy'
        'config show'
        'check object-count'
        'check disk-space'
        'status upgrade'
        'status conversion'
        'upgrade convert-master'
        'upgrade convert-primaries'
        'upgrade copy-master'
        'upgrade validate-start-cluster'
        'upgrade reconfigure-ports'
    )

    # We don't want to have to wait for the default one-second timeout for all
    # of these commands.
    export GPUPGRADE_CONNECTION_TIMEOUT=0

    # Run every subcommand.
    for command in "${commands[@]}"; do
        run gpupgrade $command
        echo "$status"
        [ "$status" -eq 1 ]
        outputContains "could not connect to the upgrade hub (did you run 'gpupgrade initialize'?)"

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade $command"
        echo "$output"

    done
}
