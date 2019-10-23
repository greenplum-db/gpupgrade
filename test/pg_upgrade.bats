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

    PSQL="$GPHOME"/bin/psql
    $PSQL -d postgres -p $PGPORT -c "DROP TABLE IF EXISTS test_pg_upgrade CASCADE;"
}

# Takes an old datadir and echoes the expected new datadir path.
upgrade_datadir() {
    local base="$(basename $1)"
    local dir="$(dirname $1)_upgrade"

    # Sanity check.
    [ -n "$base" ]
    [ -n "$dir" ]

    echo "$dir/$base"
}

setup_newmasterdir() {
    # TODO: code factor this with execute.bats
    PSQL="$GPHOME"/bin/psql

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

    run gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio=0 3>&-

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

    run gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio=0 3>&-
    [ "$status" -eq 0 ]

    NEW_CLUSTER="$newmasterdir"

    grep "Clusters are compatible" "$GPUPGRADE_HOME"/initialize.log

    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_0.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_1.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_2.log ]

    grep "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_0.log
    grep "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_1.log
    grep "Clusters are compatible" "$GPUPGRADE_HOME"/pg_upgrade_check_stdout_seg_2.log

    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_0.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_1.log ]
    [ -e "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_2.log ]

    [ ! -s "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_0.log ]
    [ ! -s "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_1.log ]
    [ ! -s "$GPUPGRADE_HOME"/pg_upgrade_check_stderr_seg_2.log ]

    KEEP_STATE_DIR=0
}
