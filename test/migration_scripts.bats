#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

SCRIPTS_DIR=$BATS_TEST_DIRNAME/../migration_scripts

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    PSQL="$GPHOME_SOURCE/bin/psql -X --no-align --tuples-only"

    backup_source_cluster "$STATE_DIR"/backup

    TEST_DBNAME=testdb
    DEFAULT_DBNAME=postgres
    GPHDFS_USER=gphdfs_user

    $PSQL -c "DROP DATABASE IF EXISTS $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -c "DROP ROLE IF EXISTS $GPHDFS_USER;" -d $DEFAULT_DBNAME

    gpupgrade kill-services
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    if [ -n "$MIGRATION_DIR" ]; then
        rm -r $MIGRATION_DIR
    fi

    gpupgrade kill-services

    restore_source_cluster "$STATE_DIR"/backup
    rm -rf "$STATE_DIR"/backup

    rm -r "$STATE_DIR"
}

# XXX backup_source_cluster is a hack to work around the standby-revert bug.
# Instead of relying on revert to correctly reset the state of the standby, copy
# over the original cluster contents during teardown.
#
# Remove this and its companion ASAP.
backup_source_cluster() {
    local backup_dir=$1

    if [[ "$MASTER_DATA_DIRECTORY" != *"/datadirs/qddir/demoDataDir-1" ]]; then
        abort "refusing to back up cluster with master '$MASTER_DATA_DIRECTORY'; demo directory layout required"
    fi

    # Don't use -p. It's important that the backup directory not exist so that
    # we know we have control over it.
    mkdir "$backup_dir"

    local datadir_root
    datadir_root="$(realpath "$MASTER_DATA_DIRECTORY"/../..)"

    gpstop -af
    # TODO: Find out why in some cases the variables used in rsync are empty/not-set
    # which causes deletion of the the root directory. Once we have identified,
    # do the necessary refactoring
    rsync --archive "${datadir_root:?}"/ "${backup_dir:?}"/
    gpstart -a
}

# XXX restore_source_cluster is a hack to work around the standby-revert bug;
# see backup_source_cluster above
restore_source_cluster() {
    local backup_dir=$1

    if [[ "$MASTER_DATA_DIRECTORY" != *"/datadirs/qddir/demoDataDir-1" ]]; then
        abort "refusing to restore cluster with master '$MASTER_DATA_DIRECTORY'; demo directory layout required"
    fi

    local datadir_root
    datadir_root="$(realpath "$MASTER_DATA_DIRECTORY"/../..)"

    stop_any_cluster
    # TODO: Find out why in some cases the variables used in rsync are empty/not-set
    # which causes deletion of the the root directory. Once we have identified,
    # do the necessary refactoring
    rsync --archive -I --delete "${backup_dir:?}"/ "${datadir_root:?}"/
    gpstart -a
}

drop_unfixable_objects() {
    # the migration script should not remove primary / unique key constraints on partitioned tables, so
    # remove them manually by dropping the table as they can't be dropped.
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE table_with_unique_constraint_p;"
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE table_with_primary_constraint_p;"
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE partition_table_partitioned_by_name_type;"
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE table_distributed_by_name_type;"
}

@test "migration scripts generate sql to modify non-upgradeable objects and fix pg_upgrade check errors" {

    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -f $BATS_TEST_DIRNAME/../migration_scripts/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose
    echo "$output"
    [ "$status" -ne 0 ] || fail "expected initialize to fail due to pg_upgrade check"

    egrep "\"CHECK_UPGRADE\": \"FAILED\"" $GPUPGRADE_HOME/status.json
    egrep "^Checking.*fatal$" $GPUPGRADE_HOME/pg_upgrade/seg-1/pg_upgrade_internal.log

    MIGRATION_DIR=`mktemp -d /tmp/migration.XXXXXX`
    "$SCRIPTS_DIR"/generate_migration_sql.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"

    drop_unfixable_objects

    root_child_indexes_before=$(get_indexes "$GPHOME_SOURCE")

    "$SCRIPTS_DIR"/execute_migration_sql.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"/pre-upgrade

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose
    gpupgrade execute --verbose
    gpupgrade finalize --verbose

    "$SCRIPTS_DIR"/execute_migration_sql.bash "$GPHOME_TARGET" "$PGPORT" "$MIGRATION_DIR"/post-upgrade

    # post-upgrade scripts should create the indexes on the target cluster
    root_child_indexes_after=$(get_indexes "$GPHOME_TARGET")

    # expect the index information to be same after the upgrade
    diff -U3 <(echo "$root_child_indexes_before") <(echo "$root_child_indexes_after")

    NEW_CLUSTER="$MASTER_DATA_DIRECTORY"
}

@test "after reverting recreate scripts must restore non-upgradeable objects" {
    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -f $BATS_TEST_DIRNAME/../migration_scripts/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    drop_unfixable_objects # don't test what we won't fix

    # Ignore the test tables that break the diff for now.
    # XXX We don't properly handle index constraints after revert, yet.
    # XXX We don't properly handle name type columns after revert, yet.
    EXCLUSIONS="-T table_with_primary_constraint "
    EXCLUSIONS+="-T table_with_unique_constraint "
    EXCLUSIONS+="-T pt_with_index "
    EXCLUSIONS+="-T sales "
    EXCLUSIONS+="-T table_with_name_as_second_column "

    MIGRATION_DIR=`mktemp -d /tmp/migration.XXXXXX`
    "$GPHOME_SOURCE"/bin/pg_dump --schema-only "$TEST_DBNAME" $EXCLUSIONS -f "$MIGRATION_DIR"/before.sql


    $SCRIPTS_DIR/generate_migration_sql.bash $GPHOME_SOURCE $PGPORT $MIGRATION_DIR
    $SCRIPTS_DIR/execute_migration_sql.bash $GPHOME_SOURCE $PGPORT $MIGRATION_DIR/pre-upgrade

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose
    gpupgrade execute --verbose
    gpupgrade revert --verbose

    $SCRIPTS_DIR/execute_migration_sql.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"/post-revert

    "$GPHOME_SOURCE"/bin/pg_dump --schema-only $TEST_DBNAME $EXCLUSIONS -f "$MIGRATION_DIR"/after.sql
    diff -U3 --speed-large-files "$MIGRATION_DIR"/before.sql "$MIGRATION_DIR"/after.sql
}

get_indexes() {
    local gphome=$1
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
         SELECT indrelid::regclass, unnest(indkey)
         FROM pg_index pi
         JOIN pg_partition pp ON pi.indrelid=pp.parrelid
         JOIN pg_class pc ON pc.oid=pp.parrelid
         ORDER by 1,2;
        "
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
        SELECT indrelid::regclass, unnest(indkey)
        FROM pg_index pi
        JOIN pg_partition_rule pp ON pi.indrelid=pp.parchildrelid
        JOIN pg_class pc ON pc.oid=pp.parchildrelid
        WHERE pc.relhassubclass='f'
        ORDER by 1,2;
    "
}
