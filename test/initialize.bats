#! /usr/bin/env bats
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers
load teardown_helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    register_teardown archive_state_dir "$STATE_DIR"

    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    TARGET_CLUSTER=

    # The process that is holding onto the port
    HELD_PORT_PID=
    AGENT_PORT=

    TARGET_PGPORT=6020

    gpupgrade kill-services
    gpupgrade initialize \
        --automatic \
        --source-gphome="${GPHOME_SOURCE}" \
        --target-gphome="${GPHOME_TARGET}" \
        --source-master-port="${PGPORT}" \
        --temp-port-range "$TARGET_PGPORT"-6040 \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
    register_teardown gpupgrade kill-services

    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    run_teardowns
}

delete_target_on_teardown() {
    register_teardown delete_target_datadirs "$(gpupgrade config show --target-datadir)"
}

setup_check_upgrade_to_fail() {
    $PSQL -d postgres -p $PGPORT -c "CREATE TABLE test_pg_upgrade(a int) DISTRIBUTED BY (a) PARTITION BY RANGE (a)(start (1) end(4) every(1));"
    $PSQL -d postgres -p $PGPORT -c "CREATE UNIQUE INDEX fomo ON test_pg_upgrade (a);"

    register_teardown teardown_check_upgrade_failure
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
        'config show'
        'execute --non-interactive'
        'revert --non-interactive'
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
            --source-gphome="$GPHOME_SOURCE" \
            --target-gphome="$GPHOME_TARGET" \
            --source-master-port="${PGPORT}" \
            --stop-before-cluster-creation \
            --automatic \
            --verbose 3>&-

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade initialize $opts ... -> $status"
        echo "$output"

        [ "$status" -eq 1 ]
        [[ $output = *'invalid argument '*' for "--disk-free-ratio" flag:'* ]] || fail
    done
}

@test "initialize skips disk space check when --disk-free-ratio is 0" {
    gpupgrade kill-services

    run gpupgrade initialize \
        --disk-free-ratio=0 \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --stop-before-cluster-creation \
        --automatic \
        --verbose 3>&-

    [[ $output != *'CHECK_DISK_SPACE'* ]] || fail "Expected disk space check to have been skipped. $output"
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
    register_teardown release_held_port
    wait_for_port_change $AGENT_PORT 0

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --automatic \
        --verbose 3>&-
    [ "$status" -ne 0 ] || fail "expected start_agent substep to fail with port already in use: $output"

    release_held_port

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --automatic \
        --verbose 3>&-
    [ "$status" -eq 0 ] || fail "expected start_agent substep to succeed: $output"
}

@test "the check_upgrade substep always runs" {
    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-

    delete_target_on_teardown
    setup_check_upgrade_to_fail

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-

    # Other substeps are skipped when marked completed in the state dir,
    # for check_upgrade, we always run it.
    [ "$status" -eq 1 ] || fail "$output"
}

@test "the source cluster is running at the end of initialize" {
    delete_target_on_teardown

    isready || fail "expected source cluster to be available"
}

@test "init target cluster is idempotent" {
    # Force a target cluster to be created (setup's initialize stops before that
    # happens).
    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-

    delete_target_on_teardown

    # To simulate an init cluster failure, stop a segment and remove a datadir
    local newmasterdir
    newmasterdir="$(gpupgrade config show --target-datadir)"
    (PGPORT=$TARGET_PGPORT source "$GPHOME_TARGET"/greenplum_path.sh && gpstart -a -d "$newmasterdir")

    local datadir=$(query_datadirs "$GPHOME_TARGET" $TARGET_PGPORT "content=1")
    pg_ctl -D "$datadir" stop
    rm -r "$datadir"

    # Ensure gpupgrade starts from initializing the target cluster.
    cat <<- EOF > "$GPUPGRADE_HOME/substeps.json"
        {
          "INITIALIZE": {
            "GENERATE_TARGET_CONFIG": "COMPLETE",
            "SAVING_SOURCE_CLUSTER_CONFIG": "COMPLETE",
            "START_AGENTS": "COMPLETE"
          }
        }
	EOF

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-
}

# This is a very simple way to flush out the most obvious idempotence bugs. It
# replicates what would happen if every substep failed/crashed right after
# completing its work but before completion was signalled back to the hub.
@test "all substeps can be re-run after completion" {
    # Force a target cluster to be created (setup's initialize stops before that
    # happens).
    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-

    delete_target_on_teardown

    # Mark every substep in the status file as failed. Then re-initialize.
    sed -i.bak -e 's/"COMPLETE"/"FAILED"/g' "$GPUPGRADE_HOME/substeps.json"

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-
}

# Regression test for 6X target clusters cross-linking against a 5X
# installation during initialize.
#
# XXX The test power here isn't very high -- it relies on the failure mode we've
# seen on Linux, which is a runtime link error printed to the gpinitsystem
# output.
@test "gpinitsystem does not run a cross-linked cluster" {
    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose 3>&-
    register_teardown gpupgrade revert --non-interactive

    echo "$output"
    [[ $output != *"libxml2.so.2: no version information available"* ]] || \
        fail "target cluster appears to be cross-linked against the source installation"
}
