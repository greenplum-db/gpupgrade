#! /usr/bin/env bats

load helpers

setup() {
    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade prepare init --old-bindir /dummy --new-bindir /dummy

    kill_hub
}

teardown() {
    kill_hub
}

@test "start-hub fails if the source configuration hasn't been initialized" {
	rm $GPUPGRADE_HOME/source_cluster_config.json
    run gpupgrade prepare start-hub
    [ "$status" -eq 1 ]

    [[ "$output" = *"Unable to load source cluster configuration"* ]]
}

@test "start-hub fails if the target configuration hasn't been initialized" {
	rm $GPUPGRADE_HOME/target_cluster_config.json
    run gpupgrade prepare start-hub
    [ "$status" -eq 1 ]

    [[ "$output" = *"Unable to load target cluster configuration"* ]]
}

@test "start-hub fails if both configurations haven't been initialized" {
	rm $GPUPGRADE_HOME/source_cluster_config.json
	rm $GPUPGRADE_HOME/target_cluster_config.json
    run gpupgrade prepare start-hub
    [ "$status" -eq 1 ]

	echo $output
    [[ "$output" = *"Unable to load source or target cluster configuration"* ]]
}

@test "start-hub finds the right hub binary and starts a daemonized process" {
    # The '3>&-' below is there because we must close fd 3 before forking new
    # processes in a BATS suite. For a full explanation, see
    #    https://github.com/bats-core/bats-core#file-descriptor-3-read-this-if-bats-hangs
    gpupgrade prepare start-hub 3>&-
    ps -ef | grep -Gq "[g]pupgrade_hub --daemon$"
}

@test "start-hub returns an error if the hub is already running" {
    gpupgrade prepare start-hub 3>&-

    # second start should return an error
    ! gpupgrade prepare start-hub
    # TODO: check for a useful error message
}

@test "start-hub does not return an error if an unrelated process has gpupgrade_hub in its name" {
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
    gpupgrade prepare start-hub 3>&-

    # Clean up. Use SIGINT rather than SIGTERM to avoid a nasty-gram from BATS.
    kill -INT $bgproc
}

@test "start-hub returns an error if gpupgrade_hub isn't on the PATH" {
    # Save the path to gpupgrade, since Bash can't look it up once we clear PATH
    GPUPGRADE=`which gpupgrade`

    ! PATH= $GPUPGRADE prepare start-hub
    # TODO: check for a useful error message
}

outputContains() {
    [[ "$output" = *"$1"* ]]
}

@test "subcommands return an error if the hub is not started" {
    commands=(
        'prepare shutdown-clusters'
        'prepare start-agents'
        'prepare init-cluster'
        'config set --old-bindir /dummy'
        'config show'
        'check version'
        'check object-count'
        'check disk-space'
        'check config'
        'check seginstall'
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

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade $command"
        echo "$output"

        [ "$status" -eq 1 ]
        outputContains "couldn't connect to the upgrade hub (did you run 'gpupgrade prepare start-hub'?)"
    done
}
