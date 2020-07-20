#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers
load tablespace_helpers
load teardown_helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    echo $GPUPGRADE_HOME

    gpupgrade kill-services

    PSQL="$GPHOME_SOURCE"/bin/psql
}


teardown() {
    skip_if_no_gpdb

    if [ -n "$TABLE" ]; then
        $PSQL postgres -c "DROP TABLE ${TABLE}"
    fi

    if [ -n "$MARKER" ]; then
        local datadirs=($(query_datadirs $GPHOME_SOURCE $PGPORT))
        for datadir in "${datadirs[@]}"; do
            rm -f "$datadir/${MARKER}"
        done
    fi

    run_teardowns
}

@test "reverting after initialize succeeds" {
    local target_hosts_dirs upgradeID

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    # grab cluster data before revert destroys it
    target_hosts_dirs=$(jq -r '.Target.Primaries[] | .DataDir' "${GPUPGRADE_HOME}/config.json")
    upgradeID=$(gpupgrade config show --id)

    gpupgrade revert --verbose

    # gpupgrade processes are stopped
    ! process_is_running "[g]pupgrade hub" || fail 'expected hub to have been stopped'
    ! process_is_running "[g]pupgrade agent" || fail 'expected agent to have been stopped'

    # target data directories are deleted
    while read -r datadir; do
        run stat "$datadir"
        ! [ $status -eq 0 ] || fail "expected datadir ${datadir} to have been deleted"
    done <<< "${target_hosts_dirs}"

    # the GPUPGRADE_HOME directory is deleted
    if [ -d "${GPUPGRADE_HOME}" ]; then
        echo "expected GPUPGRADE_HOME directory ${GPUPGRADE_HOME} to have been deleted"
        exit 1
    fi

    # check that the archived log directory corresponds to this tests upgradeID
    if [[ -z $(find "${HOME}/gpAdminLogs/gpupgrade-${upgradeID}-"* -type d) ]]; then
        fail "expected the log directory to be archived and match ${HOME}/gpAdminLogs/gpupgrade-*"
    fi
}

test_revert_after_execute() {
    local mode="$1"
    local target_master_port=6020
    local old_config new_config mirrors primaries rows

    # Save segment configuration
    old_config=$(get_segment_configuration "${GPHOME_SOURCE}")

    # Place marker files on mirrors
    MARKER=source-cluster.MARKER
    mirrors=($(query_datadirs $GPHOME_SOURCE $PGPORT "role='m'"))
    for datadir in "${mirrors[@]}"; do
        touch "$datadir/${MARKER}"
    done

    # Add a tablespace, which only works when upgrading from 5X.
    if is_GPDB5 "$GPHOME_SOURCE"; then
        local tablespace_table="tablespace_table"
        create_tablespace_with_table "$tablespace_table"
        register_teardown delete_tablespace_data "$tablespace_table"
    fi

    # Add a table
    TABLE="should_be_reverted"
    $PSQL postgres -c "CREATE TABLE ${TABLE} (a INT)"
    $PSQL postgres -c "INSERT INTO ${TABLE} VALUES (1), (2), (3)"

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range ${target_master_port}-6040 \
        --disk-free-ratio 0 \
        --mode "$mode" \
        --verbose 3>&-
    gpupgrade execute --verbose

    # Modify the table on the target cluster
    $PSQL -p $target_master_port postgres -c "TRUNCATE ${TABLE}"

    # Modify the table in the tablespace on the target cluster
    # Note: tablespace only work when upgrading from 5X.
    if is_GPDB5 "$GPHOME_SOURCE"; then
        $PSQL -p $target_master_port postgres -c "TRUNCATE $tablespace_table"
    fi

    # Revert
    gpupgrade revert --verbose

    # Verify the table modifications were reverted
    rows=$($PSQL postgres -Atc "SELECT COUNT(*) FROM ${TABLE}")
    if (( rows != 3 )); then
        fail "table ${TABLE} truncated after execute was not reverted: got $rows rows want 3"
    fi

    if is_GPDB5 "$GPHOME_SOURCE"; then
        check_tablespace_data "$tablespace_table"
    fi

    # Verify marker files on primaries
    primaries=($(query_datadirs $GPHOME_SOURCE $PGPORT "role='p'"))
    for datadir in "${primaries[@]}"; do
        if [ "$mode" = "link" ]; then
            [ -f "${datadir}/${MARKER}" ] || fail "in link mode using rsync expected ${MARKER} marker file to be in datadir: $datadir"
        else
            [ ! -f "${datadir}/${MARKER}" ] || fail "in copy mode using gprecoverseg unexpected ${MARKER} marker file in datadir: $datadir"
        fi
    done

    # Check that transactions can be started on the source
    $PSQL postgres --single-transaction -c "SELECT version()" || fail "unable to start transaction"

    # Check to make sure the old cluster still matches
    new_config=$(get_segment_configuration "${GPHOME_SOURCE}")
    [ "$old_config" = "$new_config" ] || fail "actual config: $new_config, wanted: $old_config"

    # ensure target cluster is down
    ! isready "${GPHOME_TARGET}" ${target_master_port} || fail "expected target cluster to not be running on port ${target_master_port}"

    is_source_standby_in_sync || fail "expected standby to eventually be in sync"
}

@test "reverting after execute in link mode succeeds" {
    test_revert_after_execute "link"
}

@test "reverting after execute in copy mode succeeds" {
    test_revert_after_execute "copy"
}

@test "can successfully run gpupgrade after a revert" {
    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose

    # On GPDB5, restore the primary and master directories before starting the cluster. Hack until revert handles this case
    restore_cluster

    gpupgrade revert --verbose

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose

    # This last revert is used for test cleanup.
    gpupgrade revert --verbose
}

# gp_segment_configuration does not show us the status correctly. We must check that the
# sent_location from the master equals the replay_location of the standby.
is_source_standby_in_sync() {
    local INSYNC="f"
    local duration=600 # wait up to 10 minutes
    local poll=5

    while (( duration > 0 )); do
        INSYNC=$("$PSQL" -AXt postgres -c "SELECT sent_location=replay_location FROM pg_stat_replication")

        if [[ -z "$INSYNC" ]] && ! is_source_standby_running; then
            break # standby has disappeared
        elif [[ "$INSYNC" == "t" ]]; then
            break
        fi

        sleep $poll
        (( duration = duration - poll ))
    done

    [[ $INSYNC == "t" ]]
}

is_source_standby_running() {
    local standby_datadir
    standby_datadir=$(query_datadirs "$GPHOME_SOURCE" "$PGPORT" "content = '-1' AND role = 'm'")

    if ! "${GPHOME_SOURCE}"/bin/pg_ctl status -D "$standby_datadir" > /dev/null; then
        return 1
    fi
}
