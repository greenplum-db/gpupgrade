#!/usr/bin/env bats

load helpers

setup_state_dir() {
    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

teardown_new_cluster() {
    if [ -n "$NEW_CLUSTER" ]; then
        delete_finalized_cluster $NEW_CLUSTER
    fi
}

setup() {
    skip_if_no_gpdb

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    teardown_new_cluster
    gpupgrade kill-services
    gpstart -a
}

@test "finalize brings up the standby and mirrors for the target cluster" {
    local source_mirrors_count=$(number_of_mirrors)
    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose
    gpupgrade finalize --verbose

    NEW_CLUSTER="$MASTER_DATA_DIRECTORY"

    local actual_standby_status=$(gpstate -d "${NEW_CLUSTER}")
    local standby_status_line=$(get_standby_status "$actual_standby_status")
    [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"

    local target_mirrors_count=$(number_of_mirrors)
    local gp_segment_configuration=$(psql postgres -c "select * from gp_segment_configuration")
    [[ $source_mirrors_count -eq $target_mirrors_count ]] || exit "expected target mirrors count '${target_mirrors_count}' to equal source mirrors count '${source_mirrors_count}'. gp_segment_configuration:
        ${gp_segment_configuration}"

    check_mirror_validity
}

number_of_mirrors() {
    # when the target cluster has finalized, it is running under the same PGPORT as the source cluster
    psql postgres --tuples-only --no-align -c "
        select count(*) from gp_segment_configuration
            where role='m' and status='u' and mode='s' and content != -1
    "
}

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}

# Check the validity of the upgraded mirrors - failover to them and then recover, similar to cross-subnet testing
check_mirror_validity() {
    check_can_start_transactions
    check_segments_are_synchronized
    kill_primaries
    check_can_start_transactions
    gprecoverseg -a
    check_segments_are_synchronized
    gprecoverseg -ra
    check_segments_are_synchronized
}

check_segments_are_synchronized() {
    for i in {1..10}; do
        psql -d postgres -c "SELECT gp_request_fts_probe_scan();"
        run psql -t -A -d postgres -c "SELECT count(*) FROM gp_segment_configuration WHERE content <> -1 AND mode = 'n'"
        if [ "$output" = "0" ]; then
            return 0
        fi
        sleep 5
    done

    echo "failed to synchronize within time limit"
    return 1
}

kill_primaries() {
    local primary_data_dirs=$(psql -t -A -d postgres -c "SELECT datadir FROM gp_segment_configuration WHERE content <> -1 AND role = 'p'")
    for dir in ${primary_data_dirs[@]}; do
        pg_ctl stop -m fast -D $dir -w
    done
}

check_can_start_transactions() {
    for i in {1..10}; do
        psql -d postgres -c "SELECT gp_request_fts_probe_scan();"
        run psql -t -A -d postgres -c "BEGIN; CREATE TEMP TABLE temp_test(a int) DISTRIBUTED RANDOMLY; COMMIT"
        if [[ $status -eq 0 ]]; then
            return 0
        fi
        sleep 5
    done

    echo "failed to start transactions within time limit"
    return 1
}
