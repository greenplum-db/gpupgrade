#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

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
}

@test "reverting after initialize succeeds" {
    local target_hosts_dirs upgradeID

    gpupgrade initialize \
        --source-bindir="$GPHOME_SOURCE/bin" \
        --target-bindir="$GPHOME_TARGET/bin" \
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
    local old_config new_config mirrors primaries row_count

    # Save segment configuration
    old_config=$(get_segment_configuration "${GPHOME_SOURCE}")

    # Place marker files on mirrors
    MARKER=source-cluster.MARKER
    mirrors=($(query_datadirs $GPHOME_SOURCE $PGPORT "role='m'"))
    for datadir in "${mirrors[@]}"; do
        touch "$datadir/${MARKER}"
    done

    # Add a table
    TABLE="should_be_reverted"
    $PSQL postgres -c "CREATE TABLE ${TABLE} (a INT)"
    $PSQL postgres -c "INSERT INTO ${TABLE} VALUES (1), (2), (3)"

    gpupgrade initialize \
        --source-bindir="$GPHOME_SOURCE/bin" \
        --target-bindir="$GPHOME_TARGET/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range ${target_master_port}-6040 \
        --disk-free-ratio 0 \
        --mode "$mode" \
        --verbose 3>&-
    gpupgrade execute --verbose

    # Modify the table on the target cluster
    $PSQL -p $target_master_port postgres -c "TRUNCATE ${TABLE}"

    # Revert
    gpupgrade revert --verbose

    # Verify the table modifications were reverted
    row_count=$($PSQL postgres -Atc "SELECT COUNT(*) FROM ${TABLE}")
    if (( row_count != 3 )); then
        fail "table ${TABLE} truncated after execute was not reverted: got $row_count rows want 3"
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
}

@test "reverting after execute in link mode succeeds" {
    test_revert_after_execute "link"
}

@test "reverting after execute in copy mode succeeds" {
    test_revert_after_execute "copy"
}

@test "can successfully run gpupgrade after a revert" {
    gpupgrade initialize \
        --source-bindir="$GPHOME_SOURCE/bin" \
        --target-bindir="$GPHOME_TARGET/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose

    # On GPDB5, restore the primary and master directories before starting the cluster. Hack until revert handles this case
    restore_cluster

    gpupgrade revert --verbose

    gpupgrade initialize \
        --source-bindir="$GPHOME_SOURCE/bin" \
        --target-bindir="$GPHOME_TARGET/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose

    # This last revert is used for test cleanup.
    gpupgrade revert --verbose
}
