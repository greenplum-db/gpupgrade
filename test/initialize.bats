#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    TARGET_CLUSTER=

    # The process that is holding onto the port
    HELD_PORT_PID=
    AGENT_PORT=

    gpupgrade kill-services
    gpupgrade initialize \
        --source-bindir="${GPHOME}/bin" \
        --target-bindir="${GPHOME}/bin" \
        --source-master-port="${PGPORT}"\
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

    PSQL="$GPHOME"/bin/psql
    TEARDOWN_FUNCTIONS=()
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -z "${BATS_TEST_SKIPPED}" ]; then
        gpupgrade kill-services
        rm -r "$STATE_DIR"
    fi

    for FUNCTION in "${TEARDOWN_FUNCTIONS[@]}"; do
        $FUNCTION
    done
}

set_target_cluster_var_for_teardown() {
    TARGET_CLUSTER="$(gpupgrade config show --target-datadir)"
}

teardown_target_cluster() {
    delete_target_datadirs $TARGET_CLUSTER
}

setup_check_upgrade_to_fail() {
    $PSQL -d postgres -p $PGPORT -c "CREATE TABLE test_pg_upgrade(a int) DISTRIBUTED BY (a) PARTITION BY RANGE (a)(start (1) end(4) every(1));"
    $PSQL -d postgres -p $PGPORT -c "CREATE UNIQUE INDEX fomo ON test_pg_upgrade (a);"
}

teardown_check_upgrade_failure() {
    $PSQL -d postgres -p $PGPORT -c "DROP TABLE IF EXISTS test_pg_upgrade CASCADE;"
}

release_held_port() {
    if [ -n "${HELD_PORT_PID}" ]; then
        pkill -TERM -P $HELD_PORT_PID
        wait_for_port_change $AGENT_PORT 1
        HELD_PORT_PID=
    fi
}

@test "hub daemonizes and prints the PID when passed the --daemonize option" {
    gpupgrade kill-services

    run gpupgrade hub --daemonize 3>&-
    [ "$status" -eq 0 ] || fail "$output"

    regex='pid ([[:digit:]]+)'
    [[ $output =~ $regex ]] || fail "actual output: $output"

    pid="${BASH_REMATCH[1]}"
    procname=$(ps -o ucomm= $pid)
    [ $procname = "gpupgrade" ] || fail "actual process name: $procname"
}

@test "hub fails if the configuration hasn't been initialized" {
    gpupgrade kill-services

    rm $GPUPGRADE_HOME/config.json
    run gpupgrade hub --daemonize
    [ "$status" -eq 1 ]

    [[ "$output" = *"config.json: no such file or directory"* ]]
}

@test "hub does not return an error if an unrelated process has gpupgrade hub in its name" {
    gpupgrade kill-services

    # Create a long-running process with gpupgrade hub in the name.
    exec -a "gpupgrade hub test log" sleep 5 3>&- &
    bgproc=$! # save the PID to kill later

    # Wait a little bit for the background process to get its new name.
    while ! ps -ef | grep -Gq "[g]pupgrade hub"; do
        sleep .001

        # To avoid hanging forever if something goes terribly wrong, make sure
        # the background process still exists during every iteration.
        kill -0 $bgproc
    done

    # Start the hub; there should be no errors.
    gpupgrade hub --daemonize 3>&-

    # Clean up. Use SIGINT rather than SIGTERM to avoid a nasty-gram from BATS.
    kill -INT $bgproc

    # ensure that the process is cleared. Any exit code other than 127
    # (indicating that the process didn't exist) is fine, just as long as the
    # process exits.
    wait $bgproc || [ "$?" -ne 127 ]
}

outputContains() {
    [[ "$output" = *"$1"* ]]
}

@test "subcommands return an error if the hub is not started" {
    gpupgrade kill-services

    commands=(
        'config set --source-bindir /dummy'
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
            --source-bindir="$GPHOME"/bin \
            --target-bindir="$GPHOME"/bin \
            --source-master-port="${PGPORT}" \
            --stop-before-cluster-creation \
            --verbose 3>&-

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade initialize $opts ... -> $status"
        echo "$output"

        [ "$status" -eq 1 ]
        [[ $output = *'invalid argument '*' for "--disk-free-ratio" flag:'* ]] || fail
    done
}

wait_for_port_change() {
    local port=$1
    local ret=$2
    local timeout=5

    for i in $(seq 1 $timeout);
    do
       sleep 1
       run lsof -i :$port
       if [ $status -eq $ret ]; then
           return
       fi
    done

    fail "timeout exceed when waiting for port change"
}

@test "start agents fails if a process is connected on the same TCP port" {
    # Ensure the agent is down, so that we can test port in use.
    gpupgrade kill-services
    rm -r "$GPUPGRADE_HOME"

    # squat gpupgrade agent port
    AGENT_PORT=6416
    go run ./testutils/port_listener/main.go $AGENT_PORT 3>&- &

    # Store the pid of the process group leader since the port is held by its child
    HELD_PORT_PID=$!
    TEARDOWN_FUNCTIONS+=( release_held_port )
    wait_for_port_change $AGENT_PORT 0

    run gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --verbose 3>&-
    [ "$status" -ne 0 ] || fail "expected start_agent substep to fail with port already in use: $output"

    release_held_port

    run gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --verbose 3>&-
    [ "$status" -eq 0 ] || fail "expected start_agent substep to succeed: $output"
}

@test "the check_upgrade substep always runs" {
    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    set_target_cluster_var_for_teardown
    TEARDOWN_FUNCTIONS+=( teardown_target_cluster )

    setup_check_upgrade_to_fail
    TEARDOWN_FUNCTIONS+=( teardown_check_upgrade_failure )

    run gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    # Other substeps are skipped when marked completed in the state dir,
    # for check_upgrade, we always run it.
    [ "$status" -eq 1 ] || fail "$output"
}

@test "the source cluster is running at the end of initialize" {
    set_target_cluster_var_for_teardown
    TEARDOWN_FUNCTIONS+=( teardown_target_cluster )

    pg_isready -q || fail "expected source cluster to be available"
}

# This is a very simple way to flush out the most obvious idempotence bugs. It
# replicates what would happen if every substep failed/crashed right after
# completing its work but before completion was signalled back to the hub.
@test "all substeps can be re-run after completion" {
    # Force a target cluster to be created (setup's initialize stops before that
    # happens).
    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    set_target_cluster_var_for_teardown
    TEARDOWN_FUNCTIONS+=( teardown_target_cluster )

    # Mark every substep in the status file as failed. Then re-initialize.
    sed -i.bak -e 's/"COMPLETE"/"FAILED"/g' "$GPUPGRADE_HOME/status.json"

    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-
}
