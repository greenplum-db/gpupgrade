#! /usr/bin/env bats
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers
load ../helpers/teardown_helpers

DATA_MIGRATION_INPUT_DIR=$BATS_TEST_DIRNAME/../../../data-migration-scripts

setup() {
    skip_if_no_gpdb

    [ -f "${ISOLATION2_PATH}/pg_isolation2_regress" ] || fail "Failed to find pg_isolation2_regress. Please set ISOLATION2_PATH"

    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    register_teardown archive_state_dir "$STATE_DIR"
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    DATA_MIGRATION_OUTPUT_DIR=`mktemp -d /tmp/migration.XXXXXX`
    register_teardown rm -r "$DATA_MIGRATION_OUTPUT_DIR"

    gpupgrade kill-services

    # Ensure that the cluster contains no non-upgradeable objects before the test
    # Note: This is especially important with a 5X demo cluster which contains
    # the gphdfs role by default.
    "$DATA_MIGRATION_INPUT_DIR"/gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" "$DATA_MIGRATION_OUTPUT_DIR" "$DATA_MIGRATION_INPUT_DIR"
    "$DATA_MIGRATION_INPUT_DIR"/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" "$DATA_MIGRATION_OUTPUT_DIR"/pre-initialize || true
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    run_teardowns
    gpupgrade kill-services
}

@test "pg_upgrade --check detects non-upgradeable objects" {
    local schedule="${BATS_TEST_DIRNAME}/non_upgradeable_tests/non_upgradeable_schedule"
    local tests_to_run=${NON_UPGRADEABLE_TESTS:---schedule=$schedule}

    # Note: pg_isolation2_regress requires being run from within the isolation2 directory.
    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${BATS_TEST_DIRNAME}/non_upgradeable_tests" \
            --outputdir="${BATS_TEST_DIRNAME}/non_upgradeable_tests" \
            --psqldir="${GPHOME_SOURCE}/bin" \
            --port="${PGPORT}" \
            "${tests_to_run}"
    popd
}

@test "pg_upgrade upgradeable tests" {
    # Create upgradeable objects in the source cluster
    # Note: pg_isolation2_regress requires being run from within the isolation2 directory.
    local schedule="${BATS_TEST_DIRNAME}/upgradeable_tests/source_cluster_regress/upgradeable_source_schedule"
    local tests_to_run=${UPGRADEABLE_TESTS:---schedule=$schedule}

    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${BATS_TEST_DIRNAME}/upgradeable_tests/source_cluster_regress" \
            --outputdir="${BATS_TEST_DIRNAME}/upgradeable_tests/source_cluster_regress" \
            --psqldir="${GPHOME_SOURCE}/bin" \
            --port="${PGPORT}" \
            "${tests_to_run}"
    popd

    # Upgrade the cluster
    gpupgrade initialize \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --automatic \
        --verbose
    register_teardown gpupgrade revert --non-interactive --verbose
    gpupgrade execute --non-interactive --verbose

    # Assert that upgradeable objects have been upgraded against the target cluster.
    # Note: --use-existing is needed to use the isolation2test database
    # created as a result of running the source cluster tests.
    schedule="${BATS_TEST_DIRNAME}/upgradeable_tests/target_cluster_regress/upgradeable_target_schedule"
    tests_to_run=${UPGRADEABLE_TESTS:---schedule=$schedule}

    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${BATS_TEST_DIRNAME}/upgradeable_tests/target_cluster_regress" \
            --outputdir="${BATS_TEST_DIRNAME}/upgradeable_tests/target_cluster_regress" \
            --use-existing \
            --psqldir="${GPHOME_TARGET}/bin" \
            --port="$(gpupgrade config show --target-port)" \
            "${tests_to_run}"
    popd
}
