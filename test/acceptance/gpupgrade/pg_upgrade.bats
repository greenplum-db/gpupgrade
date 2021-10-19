#! /usr/bin/env bats
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    export GPUPGRADE_LOGDIR=~/gpAdminLogs/gpupgrade
    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
    KEEP_STATE_DIR=1

    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    if [ $KEEP_STATE_DIR -eq 0 ]; then
        archive_state_dir "$STATE_DIR"
    else
        echo "state dir: $STATE_DIR"
    fi

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi

    start_source_cluster

    $PSQL -d postgres -p $PGPORT -c "DROP TABLE IF EXISTS test_pg_upgrade CASCADE;"
}

# yes, this will fail once we allow an index on a partition table
@test "pg_upgrade --check fails on a source cluster with an index on a partition table" {
    skip_if_no_gpdb

    # add in a index on a partition table, which causes pg_upgrade --check to fail
    $PSQL -d postgres -p $PGPORT -c "CREATE TABLE test_pg_upgrade(a int) DISTRIBUTED BY (a) PARTITION BY RANGE (a)(start (1) end(4) every(1));"
    $PSQL -d postgres -p $PGPORT -c "CREATE UNIQUE INDEX fomo ON test_pg_upgrade (a);"

    # Use --verbose to help debug cases where the grep fails. run() will hide
    # that output, so manually store the status and ignore the expected failure.
    local status=0
    gpupgrade initialize \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "$PGPORT" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio=0 \
        --automatic \
        --verbose 3>&- || status=$?
    [ "$status" -eq 1 ]

    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    grep "Checking for indexes on partitioned tables                  fatal" "$GPUPGRADE_LOGDIR"/initialize_*.log

    # revert added index
    $PSQL -d postgres -p $PGPORT -c "DROP TABLE test_pg_upgrade CASCADE;"

    KEEP_STATE_DIR=0
}

@test "gpupgrade initialize runs pg_upgrade --check on master and primaries" {
    skip_if_no_gpdb

    gpupgrade initialize \
        --automatic \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "$PGPORT" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio=0 3>&-

    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    grep "Clusters are compatible" "$GPUPGRADE_LOGDIR"/initialize_*.log

    [ -e ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/pg_upgrade_internal.log ]
    [ -e ~/gpAdminLogs/gpupgrade/pg_upgrade/p0/pg_upgrade_internal.log ]
    [ -e ~/gpAdminLogs/gpupgrade/pg_upgrade/p1/pg_upgrade_internal.log ]
    [ -e ~/gpAdminLogs/gpupgrade/pg_upgrade/p2/pg_upgrade_internal.log ]

    grep -c "Clusters are compatible" ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/pg_upgrade_internal.log
    grep -c "Clusters are compatible" ~/gpAdminLogs/gpupgrade/pg_upgrade/p0/pg_upgrade_internal.log
    grep -c "Clusters are compatible" ~/gpAdminLogs/gpupgrade/pg_upgrade/p1/pg_upgrade_internal.log
    grep -c "Clusters are compatible" ~/gpAdminLogs/gpupgrade/pg_upgrade/p2/pg_upgrade_internal.log

    KEEP_STATE_DIR=0
}

# Prints the number of unique primary gp_dbids on a system, as indicated by the
# gp_dbid GUCs actually stored on each segment, NOT the gp_segment_configuration
# stored on the master.
count_primary_gp_dbids() {
    local gphome=$1
    local port=$2

    for datadir in $($(query_datadirs $GPHOME_SOURCE $PGPORT "role = 'p'")); do
        "$gphome"/bin/postgres -C gp_dbid -D $datadir
    done | sort | uniq | wc -l
}

@test "upgrade maintains separate DBIDs for each segment" {
    setup_restore_cluster "--mode=copy"

    local old_dbid_num=$(count_primary_gp_dbids $GPHOME_SOURCE $PGPORT)

    gpupgrade initialize \
        --automatic \
        --verbose \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "$PGPORT" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio=0 3>&-
    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    gpupgrade execute --non-interactive --verbose

    local new_dbid_num=$(count_primary_gp_dbids $GPHOME_TARGET 6020)

    [ $old_dbid_num -eq $new_dbid_num ] || fail "expected $old_dbid_num distinct DBIDs; got $new_dbid_num"

    KEEP_STATE_DIR=0

    restore_cluster
}
