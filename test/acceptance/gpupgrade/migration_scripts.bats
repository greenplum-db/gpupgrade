#! /usr/bin/env bats
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers
load ../helpers/teardown_helpers

SCRIPTS_DIR=$BATS_TEST_DIRNAME/../../../data-migration-scripts

setup() {
    skip_if_no_gpdb

    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    register_teardown archive_state_dir "$STATE_DIR"

    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    backup_source_cluster "$STATE_DIR"/backup

    TEST_SCHEMA=testschema # must match what is used in create_nonupgradable_objects.sql
    TEST_DBNAME=testdb
    DEFAULT_DBNAME=postgres
    GPHDFS_USER=gphdfs_user

    PSQL="$GPHOME_SOURCE/bin/psql -X --no-align --tuples-only"

    $PSQL -c "DROP DATABASE IF EXISTS $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -c "DROP ROLE IF EXISTS $GPHDFS_USER;" -d $DEFAULT_DBNAME

}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    gpupgrade kill-services

    run_teardowns
}

@test "migration scripts generate sql to modify non-upgradeable objects and fix pg_upgrade check errors" {

    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    PGOPTIONS='--client-min-messages=warning' $PSQL -f "$SCRIPTS_DIR"/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    run gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose
    echo "$output"
    [ "$status" -ne 0 ] || fail "expected initialize to fail due to pg_upgrade check"

    egrep "\"CHECK_UPGRADE\": \"FAILED\"" $GPUPGRADE_HOME/substeps.json
    egrep "^Checking.*fatal$" ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/pg_upgrade_internal.log

    PGOPTIONS='--client-min-messages=warning' $PSQL -d $TEST_DBNAME -f "$SCRIPTS_DIR"/test/drop_unfixable_objects.sql

    root_child_indexes_before=$(get_indexes "$GPHOME_SOURCE")
    tsquery_datatype_objects_before=$(get_tsquery_datatypes "$GPHOME_SOURCE")
    name_datatype_objects_before=$(get_name_datatypes "$GPHOME_SOURCE")
    fk_constraints_before=$(get_fk_constraints "$GPHOME_SOURCE")
    primary_unique_constraints_before=$(get_primary_unique_constraints "$GPHOME_SOURCE")

    MIGRATION_DIR=`mktemp -d /tmp/migration.XXXXXX`
    "$SCRIPTS_DIR"/gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR" "$SCRIPTS_DIR"
    "$SCRIPTS_DIR"/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"/pre-initialize

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose
    gpupgrade execute --non-interactive --verbose
    gpupgrade finalize --non-interactive --verbose

    (source "${GPHOME_TARGET}"/greenplum_path.sh && "${GPHOME_TARGET}"/bin/gpstart -a)

    "$SCRIPTS_DIR"/gpupgrade-migration-sql-executor.bash "$GPHOME_TARGET" "$PGPORT" "$MIGRATION_DIR"/post-finalize

    # migration scripts should create the indexes on the target cluster
    root_child_indexes_after=$(get_indexes "$GPHOME_TARGET")
    tsquery_datatype_objects_after=$(get_tsquery_datatypes "$GPHOME_TARGET")
    name_datatype_objects_after=$(get_name_datatypes "$GPHOME_TARGET")
    fk_constraints_after=$(get_fk_constraints "$GPHOME_TARGET")
    primary_unique_constraints_after=$(get_primary_unique_constraints "$GPHOME_TARGET")

    # expect the index and tsquery datatype information to be same after the upgrade
    diff -U3 <(echo "$root_child_indexes_before") <(echo "$root_child_indexes_after")
    diff -U3 <(echo "$tsquery_datatype_objects_before") <(echo "$tsquery_datatype_objects_after")
    diff -U3 <(echo "$name_datatype_objects_before") <(echo "$name_datatype_objects_after")
    diff -U3 <(echo "$fk_constraints_before") <(echo "$fk_constraints_after")
    diff -U3 <(echo "$primary_unique_constraints_before") <(echo "$primary_unique_constraints_after")
}

@test "after reverting recreate scripts must restore non-upgradeable objects" {
    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -f "$SCRIPTS_DIR"/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    $PSQL -d $TEST_DBNAME -f "$SCRIPTS_DIR"/test/drop_unfixable_objects.sql

    root_child_indexes_before=$(get_indexes "$GPHOME_SOURCE")
    tsquery_datatype_objects_before=$(get_tsquery_datatypes "$GPHOME_SOURCE")
    name_datatype_objects_before=$(get_name_datatypes "$GPHOME_SOURCE")
    fk_constraints_before=$(get_fk_constraints "$GPHOME_SOURCE")
    primary_unique_constraints_before=$(get_primary_unique_constraints "$GPHOME_SOURCE")

    MIGRATION_DIR=`mktemp -d /tmp/migration.XXXXXX`
    register_teardown rm -r "$MIGRATION_DIR"

    $SCRIPTS_DIR/gpupgrade-migration-sql-generator.bash $GPHOME_SOURCE $PGPORT $MIGRATION_DIR "$SCRIPTS_DIR"
    $SCRIPTS_DIR/gpupgrade-migration-sql-executor.bash $GPHOME_SOURCE $PGPORT $MIGRATION_DIR/pre-initialize

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose
    gpupgrade execute --non-interactive --verbose
    gpupgrade revert --non-interactive --verbose

    $SCRIPTS_DIR/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"/post-revert

    # migration scripts should create the indexes on the target cluster
    root_child_indexes_after=$(get_indexes "$GPHOME_SOURCE")
    tsquery_datatype_objects_after=$(get_tsquery_datatypes "$GPHOME_SOURCE")
    name_datatype_objects_after=$(get_name_datatypes "$GPHOME_SOURCE")
    fk_constraints_after=$(get_fk_constraints "$GPHOME_SOURCE")
    primary_unique_constraints_after=$(get_primary_unique_constraints "$GPHOME_SOURCE")

    # expect the index and tsquery datatype information to be same after the upgrade
    diff -U3 <(echo "$root_child_indexes_before") <(echo "$root_child_indexes_after")
    diff -U3 <(echo "$tsquery_datatype_objects_before") <(echo "$tsquery_datatype_objects_after")
    diff -U3 <(echo "$name_datatype_objects_before") <(echo "$name_datatype_objects_after")
    diff -U3 <(echo "$fk_constraints_before") <(echo "$fk_constraints_after")
    diff -U3 <(echo "$primary_unique_constraints_before") <(echo "$primary_unique_constraints_after")
}

@test "migration scripts ignore .psqlrc files" {
    # 5X doesn't support the PSQLRC envvar we need to avoid destroying the dev
    # environment.
    if is_GPDB5 "$GPHOME_SOURCE"; then
        skip "GPDB 5 does not support alternative PSQLRC locations"
    fi

    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -f "$SCRIPTS_DIR"/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    MIGRATION_DIR=$(mktemp -d /tmp/migration.XXXXXX)
    register_teardown rm -r "$MIGRATION_DIR"

    # Set up psqlrc to kill any psql processes as soon as they're started.
    export PSQLRC="$MIGRATION_DIR"/psqlrc
    printf '\! kill $PPID\n' > "$PSQLRC"

    "$SCRIPTS_DIR"/gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR" "$SCRIPTS_DIR"
    $PSQL -d $TEST_DBNAME -f "$SCRIPTS_DIR"/test/drop_unfixable_objects.sql
    "$SCRIPTS_DIR"/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" "$MIGRATION_DIR"/pre-initialize

    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --automatic \
        --verbose
    gpupgrade revert --non-interactive --verbose

    "$SCRIPTS_DIR"/gpupgrade-migration-sql-executor.bash "$GPHOME_TARGET" "$PGPORT" "$MIGRATION_DIR"/post-revert
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

get_tsquery_datatypes() {
    local gphome=$1
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
        SELECT n.nspname, c.relname, a.attname
        FROM pg_catalog.pg_class c,
             pg_catalog.pg_namespace n,
            pg_catalog.pg_attribute a
        WHERE c.relkind = 'r'
        AND c.oid = a.attrelid
        AND NOT a.attisdropped
        AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
        AND c.relnamespace = n.oid
        AND n.nspname !~ '^pg_temp_'
        AND n.nspname !~ '^pg_toast_temp_'
        AND n.nspname NOT IN ('pg_catalog',
                                'information_schema')
        AND c.oid NOT IN
            (SELECT DISTINCT parchildrelid
            FROM pg_catalog.pg_partition_rule)
        ORDER BY 1,2,3;
        "
}

get_name_datatypes() {
    local gphome=$1
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
        SELECT n.nspname, c.relname, a.attname
        FROM pg_catalog.pg_class c,
             pg_catalog.pg_namespace n,
            pg_catalog.pg_attribute a
        WHERE c.relkind = 'r'
        AND c.oid = a.attrelid
        AND NOT a.attisdropped
        AND a.atttypid = 'pg_catalog.name'::pg_catalog.regtype
        AND c.relnamespace = n.oid
        AND n.nspname !~ '^pg_temp_'
        AND n.nspname !~ '^pg_toast_temp_'
        AND n.nspname NOT IN ('pg_catalog',
                                'information_schema')
        AND c.oid NOT IN
            (SELECT DISTINCT parchildrelid
            FROM pg_catalog.pg_partition_rule)
        ORDER BY 1,2,3;
        "
}

get_fk_constraints() {
    local gphome=$1
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
        SELECT nspname, relname, conname
        FROM pg_constraint cc
            JOIN
            (
            SELECT DISTINCT
                c.oid,
                n.nspname,
                c.relname
            FROM
                pg_catalog.pg_partition p
            JOIN
                pg_catalog.pg_class c
                ON (p.parrelid = c.oid)
            JOIN
                pg_catalog.pg_namespace n
                ON (n.oid = c.relnamespace)
            ) AS sub
            ON sub.oid = cc.conrelid
        WHERE
            cc.contype = 'f'
        ORDER BY 1,2,3;
        "
}

get_primary_unique_constraints() {
    local gphome=$1
    $gphome/bin/psql -d "$TEST_DBNAME" -p "$PGPORT" -Atc "
    WITH CTE as
    (
        SELECT oid, *
        FROM pg_class
        WHERE
            oid NOT IN
            (
                SELECT DISTINCT
                    parchildrelid
                FROM
                    pg_partition_rule
            )
    )
    SELECT
        n.nspname, cc.relname, conname
    FROM
        pg_constraint con
        JOIN
        pg_depend dep
        ON (refclassid, classid, objsubid) =
        (
           'pg_constraint'::regclass,
           'pg_class'::regclass,
           0
        )
        AND refobjid = con.oid
        AND deptype = 'i'
        -- Note that 'x' is an option for contype in GPDB6, and not in GPDB5
        -- It is included in this query to make it compatible for both.
        AND contype IN ('u', 'p', 'x')
        JOIN
        CTE c
        ON objid = c.oid
        AND relkind = 'i'
        JOIN
        CTE cc
        ON cc.oid = con.conrelid
        JOIN
        pg_namespace n
        ON (n.oid = cc.relnamespace)
    ORDER BY 1,2,3;
    "
}
