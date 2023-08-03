#! /usr/bin/env bats
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers/helpers
load helpers/teardown_helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    register_teardown archive_state_dir "$STATE_DIR"

    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    # If this variable is set (to a coordinator data directory), teardown() will call
    # gpdeletesystem on this cluster.
    TARGET_CLUSTER=

    # The process that is holding onto the port
    HELD_PORT_PID=
    AGENT_PORT=

    TARGET_PGPORT=6020

    run gpupgrade kill-services

    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    run gpupgrade kill-services
    run_teardowns
}

setup_check_upgrade_to_fail() {
    $PSQL -v ON_ERROR_STOP=1 -d postgres -p $PGPORT -c "CREATE TABLE test_pg_upgrade(a int) DISTRIBUTED BY (a) PARTITION BY RANGE (a)(start (1) end(4) every(1));"
    $PSQL -v ON_ERROR_STOP=1 -d postgres -p $PGPORT -c "CREATE UNIQUE INDEX fomo ON test_pg_upgrade (a);"

    register_teardown teardown_check_upgrade_failure
}

teardown_check_upgrade_failure() {
    $PSQL -v ON_ERROR_STOP=1 -d postgres -p $PGPORT -c "DROP TABLE IF EXISTS test_pg_upgrade CASCADE;"
}

release_held_port() {
    if [ -n "${HELD_PORT_PID}" ]; then
        pkill -TERM -P $HELD_PORT_PID
        wait_for_port_change $AGENT_PORT 1
        HELD_PORT_PID=
    fi
}

@test "hub daemonizes and prints the PID when passed the --daemonize option" {
    run gpupgrade initialize \
        --non-interactive \
        --source-gphome="${GPHOME_SOURCE}" \
        --target-gphome="${GPHOME_TARGET}" \
        --source-master-port="${PGPORT}" \
        --temp-port-range "$TARGET_PGPORT"-6040 \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

    run gpupgrade kill-services

    run gpupgrade hub --daemonize 3>&-
    [ "$status" -eq 0 ] || fail "$output"

    regex='pid ([[:digit:]]+)'
    [[ $output =~ $regex ]] || fail "actual output: $output"

    pid="${BASH_REMATCH[1]}"
    procname=$(ps -o ucomm= $pid)
    [ $procname = "gpupgrade" ] || fail "actual process name: $procname"
}

@test "hub fails if the configuration hasn't been initialized" {
    run gpupgrade initialize \
        --non-interactive \
        --source-gphome="${GPHOME_SOURCE}" \
        --target-gphome="${GPHOME_TARGET}" \
        --source-master-port="${PGPORT}" \
        --temp-port-range "$TARGET_PGPORT"-6040 \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

    run gpupgrade kill-services

    rm $GPUPGRADE_HOME/config.json
    run gpupgrade hub --daemonize
    [ "$status" -eq 1 ]

    [[ "$output" = *"config.json: no such file or directory"* ]]
}

@test "hub does not return an error if an unrelated process has gpupgrade hub in its name" {
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
    run gpupgrade initialize \
        --non-interactive \
        --source-gphome="${GPHOME_SOURCE}" \
        --target-gphome="${GPHOME_TARGET}" \
        --source-master-port="${PGPORT}" \
        --temp-port-range "$TARGET_PGPORT"-6040 \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

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
    run gpupgrade initialize \
        --non-interactive \
        --source-gphome="${GPHOME_SOURCE}" \
        --target-gphome="${GPHOME_TARGET}" \
        --source-master-port="${PGPORT}" \
        --temp-port-range "$TARGET_PGPORT"-6040 \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

    run gpupgrade kill-services

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
        outputContains 'Try restarting the hub with "gpupgrade restart-services".'
    done
}

@test "initialize fails when passed invalid --disk-free-ratio values" {
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
            --non-interactive \
            --verbose 3>&-

        # Trace which command we're on to make debugging easier.
        echo "\$ gpupgrade initialize $opts ... -> $status"
        echo "$output"

        [ "$status" -eq 1 ]
        [[ $output = *'invalid argument '*' for "--disk-free-ratio" flag:'* ]] || fail
    done
}

@test "initialize skips disk space check when --disk-free-ratio is 0" {
    run gpupgrade initialize \
        --disk-free-ratio=0 \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --stop-before-cluster-creation \
        --non-interactive \
        --verbose 3>&-

    [[ $output != *'CHECK_DISK_SPACE'* ]] || fail "Expected disk space check to have been skipped. $output"
}

@test "fails when temp-port-range overlaps with source cluster ports" {
    run gpupgrade initialize \
        --disk-free-ratio 0 \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "${PGPORT}" \
        --temp-port-range "${PGPORT}-$(($PGPORT + 20))" \
        --non-interactive \
        --verbose 3>&-

    [ "$status" -eq 1 ] || fail
    echo $output
    [[ $output = *"temp_port_range contains port"*"which overlaps with the source cluster ports on host $(hostname). Specify a non-overlapping temp_port_range."* ]] || fail
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
        --non-interactive \
        --verbose 3>&-
    [ "$status" -ne 0 ] || fail "expected start_agent substep to fail with port already in use: $output"
    [[ $output = *'"start_agents": exit status 1'* ]] || fail "expected start_agent substep to fail with port already in use: $output"

    release_held_port
    run gpupgrade revert --non-interactive --verbose

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --non-interactive \
        --verbose 3>&-
    [ "$status" -eq 0 ] || fail "expected start_agent substep to succeed: $output"
}

@test "the check_upgrade substep always runs" {
    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    setup_check_upgrade_to_fail

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    # Other substeps are skipped when marked completed in the state dir,
    # for check_upgrade, we always run it.
    [ "$status" -eq 1 ] || fail "$output"

    run gpupgrade revert --non-interactive --verbose
}

@test "the source cluster is running at the end of initialize" {
    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    isready || fail "expected source cluster to be available"

    run gpupgrade revert --non-interactive --verbose
}

@test "init target cluster is idempotent" {
    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    # To simulate an init cluster failure, stop a segment and remove a datadir
    local new_coordinator_dir
    new_coordinator_dir="$(gpupgrade config show --target-datadir)"
    # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    (unset LD_LIBRARY_PATH; PGPORT=$TARGET_PGPORT source "$GPHOME_TARGET"/greenplum_path.sh && gpstart -a -d "$new_coordinator_dir")

    local datadir=$(query_datadirs "$GPHOME_TARGET" $TARGET_PGPORT "content=1")
    run pg_ctl -D "$datadir" stop
    run rm -r "$datadir"

    # Ensure gpupgrade starts from initializing the target cluster.
    cat <<- EOF > "$GPUPGRADE_HOME/substeps.json"
        {
          "initialize": {
            "generate_target_config": "complete",
            "saving_source_cluster_config": "complete",
            "start_agents": "complete"
          }
        }
	EOF

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    run gpupgrade revert --non-interactive --verbose
}

# This is a very simple way to flush out the most obvious idempotence bugs. It
# replicates what would happen if every substep failed/crashed right after
# completing its work but before completion was signalled back to the hub.
@test "all substeps can be re-run after completion" {
    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    # Mark every substep in the status file as failed. Then re-initialize.
    sed -i.bak -e 's/"complete"/"failed"/g' "$GPUPGRADE_HOME/substeps.json"

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --non-interactive \
        --verbose 3>&-

    run gpupgrade revert --non-interactive --verbose
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
        --non-interactive \
        --verbose 3>&-

    echo "$output"
    [[ $output != *"libxml2.so.2: no version information available"* ]] || \
        fail "target cluster appears to be cross-linked against the source installation"

    run gpupgrade revert --non-interactive --verbose
}
