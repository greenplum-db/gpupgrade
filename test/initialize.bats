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
        --old-bindir="$PWD" \
        --new-bindir="$PWD" \
        --old-port="${PGPORT}"\
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -z "${BATS_TEST_SKIPPED}" ]; then
        gpupgrade kill-services
        rm -r "$STATE_DIR"
    fi
}

@test "hub saves cluster configs to disk when initialized" {
    # XXX how useful is a test for this behavior?
    [ -f "$GPUPGRADE_HOME"/source_cluster_config.json ]
    [ -f "$GPUPGRADE_HOME"/target_cluster_config.json ]
}

@test "hub daemonizes and prints the PID when passed the --daemonize option" {
    gpupgrade kill-services

    run gpupgrade_hub --daemonize 3>&-
    [ "$status" -eq 0 ] || fail "$output"

    regex='pid ([[:digit:]]+)'
    [[ $output =~ $regex ]] || fail "actual output: $output"

    pid="${BASH_REMATCH[1]}"
    procname=$(ps -o ucomm= $pid)
    [ $procname = gpupgrade_hub ] || fail "actual process name: $procname"
}

@test "hub fails if the source configuration hasn't been initialized" {
    gpupgrade kill-services

    rm $GPUPGRADE_HOME/source_cluster_config.json
    run gpupgrade_hub --daemonize
    [ "$status" -eq 1 ]

    [[ "$output" = *"Unable to load source cluster configuration"* ]]
}

@test "hub fails if the target configuration hasn't been initialized" {
    gpupgrade kill-services

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

@test "hub does not return an error if an unrelated process has gpupgrade_hub in its name" {
    gpupgrade kill-services

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
    gpupgrade kill-services

    commands=(
        'config set --old-bindir /dummy'
        'config show'
        'execute'
        'finalize'
    )

    # We don't want to have to wait for the default one-second timeout for all
    # of these commands.
    export GPUPGRADE_CONNECTION_TIMEOUT=0

    # Run every subcommand.
    for command in "${commands[@]}"; do
        run gpupgrade $command

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade $command -> $status"
        echo "$output"

        [ "$status" -eq 1 ]
        outputContains "could not connect to the upgrade hub (did you run 'gpupgrade initialize'?)"
    done
}

@test "initialize fails when passed invalid --disk-free-ratio values" {
    gpupgrade kill-services

    option_list=(
        '--disk-free-ratio=1.5'
        '--disk-free-ratio=-0.5'
        '--disk-free-ratio=abcd'
    )

    for opts in "${option_list[@]}"; do
        run gpupgrade initialize \
            $opts \
            --old-bindir="$GPHOME"/bin \
            --new-bindir="$GPHOME"/bin \
            --old-port="${PGPORT}" \
            --stop-before-cluster-creation 3>&-

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade initialize $opts ... -> $status"
        echo "$output"

        [ "$status" -eq 1 ]
        [[ $output = *'invalid argument '*' for "--disk-free-ratio" flag:'* ]] || fail
    done
}
