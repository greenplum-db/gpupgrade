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
    tmpdir=`mktemp -d`
    tmpfile="${tmpdir}/gpupgrade_hub_test_log"
    touch $tmpfile
    tail -f $tmpfile 3>&- & # we'll `kill %1` this process later
    ps -ef | grep -Gq "[g]pupgrade_hub" # double-check that it's actually there

    # Start the hub; there should be no errors.
    gpupgrade prepare start-hub 3>&-

    # Clean up.
    kill %1
    rm $tmpfile
    rm -r $tmpdir
}

@test "start-hub returns an error if gpupgrade_hub isn't on the PATH" {
    # Save the path to gpupgrade, since Bash can't look it up once we clear PATH
    GPUPGRADE=`which gpupgrade`

    ! PATH= $GPUPGRADE prepare start-hub
    # TODO: check for a useful error message
}
