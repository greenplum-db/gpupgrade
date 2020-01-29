#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
    KEEP_STATE_DIR=1

    PSQL="$GPHOME"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    if [ $KEEP_STATE_DIR -eq 0 ]; then
        rm -r "$STATE_DIR"
    else
        echo "state dir: $STATE_DIR"
    fi

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $NEW_CLUSTER
    fi

    gpstart -a

    $PSQL -d postgres -p $PGPORT -c "DROP TABLE IF EXISTS test_pg_upgrade CASCADE;"
}

setup_newmasterdir() {
    # TODO: code factor this with execute.bats
    run $PSQL -At -p $PGPORT postgres -c "SELECT datadir FROM gp_segment_configuration WHERE role = 'p' and content = -1"
    [ "$status" -eq 0 ] || fail "$output"

    newmasterdir="$(upgrade_datadir $output)"
}

# yes, this will fail once we allow an index on a partition table
@test "pg_upgrade --check fails on a source cluster with an index on a partition table" {
    skip_if_no_gpdb
    setup_newmasterdir

    # add in a index on a partition table, which causes pg_upgrade --check to fail
    $PSQL -d postgres -p $PGPORT -c "CREATE TABLE test_pg_upgrade(a int) DISTRIBUTED BY (a) PARTITION BY RANGE (a)(start (1) end(4) every(1));"
    $PSQL -d postgres -p $PGPORT -c "CREATE UNIQUE INDEX fomo ON test_pg_upgrade (a);"

    # Use --verbose to help debug cases where the grep fails. run() will hide
    # that output, so manually store the status and ignore the expected failure.
    local status=0
    gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio=0 \
        --verbose 3>&- || status=$?
    [ "$status" -eq 1 ]

    NEW_CLUSTER="$newmasterdir"

    grep "Checking for indexes on partitioned tables                  fatal" "$GPUPGRADE_HOME"/initialize.log

    # revert added index
    gpstart -a
    $PSQL -d postgres -p $PGPORT -c "DROP TABLE test_pg_upgrade CASCADE;"
    gpstop -a

    KEEP_STATE_DIR=0
}

@test "gpupgrade initialize runs pg_upgrade --check on master and primaries" {
    skip_if_no_gpdb
    setup_newmasterdir

    gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio=0 3>&-

    NEW_CLUSTER="$newmasterdir"

    grep "Clusters are compatible" "$GPUPGRADE_HOME"/initialize.log

    [ -e "$GPUPGRADE_HOME"/pg_upgrade/seg-1/pg_upgrade_internal.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade/seg0/pg_upgrade_internal.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade/seg1/pg_upgrade_internal.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade/seg2/pg_upgrade_internal.log ]

    grep -c "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade/seg-1/pg_upgrade_internal.log
    grep -c "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade/seg0/pg_upgrade_internal.log
    grep -c "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade/seg1/pg_upgrade_internal.log
    grep -c "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade/seg2/pg_upgrade_internal.log

    KEEP_STATE_DIR=0
}

# Prints the number of unique primary gp_dbids on a system, as indicated by the
# gp_dbid GUCs actually stored on each segment, NOT the gp_segment_configuration
# stored on the master.
count_primary_gp_dbids() {
    local port=$1

    for datadir in $($PSQL -At -p $port postgres -c "
        select datadir from gp_segment_configuration where role='p'
    "); do
        "$GPHOME"/bin/postgres -C gp_dbid -D $datadir
    done | sort | uniq | wc -l
}

@test "upgrade maintains separate DBIDs for each segment" {
    local old_dbid_num=$(count_primary_gp_dbids $PGPORT)

    setup_newmasterdir
    gpupgrade initialize \
        --verbose \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio=0 3>&-
    NEW_CLUSTER="$newmasterdir"

    gpupgrade execute --verbose

    local new_dbid_num=$(count_primary_gp_dbids 50432)

    [ $old_dbid_num -eq $new_dbid_num ] || fail "expected $old_dbid_num distinct DBIDs; got $new_dbid_num"

    KEEP_STATE_DIR=0
}
