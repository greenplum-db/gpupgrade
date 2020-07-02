#!/usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

PREUPGRADE_SCRIPTS_DIR=$BATS_TEST_DIRNAME/../migration_scripts/pre-upgrade

setup() {
    skip_if_no_gpdb

    PSQL="$GPHOME_SOURCE/bin/psql -X --no-align --tuples-only"

    TEST_DBNAME=testdb
    DEFAULT_DBNAME=postgres
    GPHDFS_USER=gphdfs_user

    $PSQL -c "DROP DATABASE IF EXISTS $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -c "DROP ROLE IF EXISTS $GPHDFS_USER;" -d $DEFAULT_DBNAME

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services
}

teardown() {
    skip_if_no_gpdb

    if [ -n "$NEW_CLUSTER" ]; then
        delete_finalized_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi

    gpupgrade kill-services

    restore_cluster

    start_source_cluster

    $GPHOME_SOURCE/bin/psql -c "DROP ROLE IF EXISTS ${GPHDFS_USER}" -d $DEFAULT_DBNAME
    $GPHOME_SOURCE/bin/psql -c "DROP DATABASE IF EXISTS ${TEST_DBNAME}" -d $DEFAULT_DBNAME
}

@test "migration scripts generate sql to modify non-upgradeable objects and fix pg_upgrade check errors" {
    setup_restore_cluster "--mode=copy"

    $PSQL -c "CREATE DATABASE $TEST_DBNAME;" -d $DEFAULT_DBNAME
    $PSQL -f $BATS_TEST_DIRNAME/../migration_scripts/test/create_nonupgradable_objects.sql -d $TEST_DBNAME

    run gpupgrade initialize \
            --source-gphome="$GPHOME_SOURCE" \
            --target-gphome="$GPHOME_TARGET" \
            --source-master-port="${PGPORT}" \
            --temp-port-range 6020-6040 \
            --disk-free-ratio 0 \
            --verbose
    [ "$status" -ne 0 ] || fail "expected initialize to fail due to pg_upgrade check: $output"

    egrep "\"CHECK_UPGRADE\": \"FAILED\"" $GPUPGRADE_HOME/status.json
    egrep "^Checking.*fatal$" $GPUPGRADE_HOME/pg_upgrade/seg-1/pg_upgrade_internal.log

    PREUPGRADE_DIR=`mktemp -d /tmp/migration.XXXXXX`
    $PREUPGRADE_SCRIPTS_DIR/generate_preupgrade_sql.bash $GPHOME_SOURCE $PGPORT $PREUPGRADE_DIR
    $PREUPGRADE_SCRIPTS_DIR/execute_preupgrade_sql.bash $GPHOME_SOURCE $PGPORT $PREUPGRADE_DIR

    # the migration script should not remove primary / unique key constraints on partitioned tables, so
    # remove them manually by dropping the table as they can't be dropped.
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE table_with_unique_constraint_p;"
    $GPHOME_SOURCE/bin/psql -d $TEST_DBNAME -p $PGPORT -c "DROP TABLE table_with_primary_constraint_p;"

    gpupgrade initialize \
            --source-gphome="$GPHOME_SOURCE" \
            --target-gphome="$GPHOME_TARGET" \
            --source-master-port="${PGPORT}" \
            --temp-port-range 6020-6040 \
            --disk-free-ratio 0 \
            --verbose
    gpupgrade execute --verbose
    gpupgrade finalize --verbose

    NEW_CLUSTER="$MASTER_DATA_DIRECTORY"
}
