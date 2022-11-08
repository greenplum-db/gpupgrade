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

    # Set PYTHONPATH directly since it is needed when running the pg_upgrade tests locally. Normally one would source
    # greenplum_path.sh, but that causes the following issues:
    # https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    export PYTHONPATH=${GPHOME_TARGET}/lib/python

    export TEST_DIR="${BATS_TEST_DIRNAME}/6-to-7"
    if is_GPDB5 "$GPHOME_SOURCE"; then
        export TEST_DIR="${BATS_TEST_DIRNAME}/5-to-6"
    fi

    # Ensure that the cluster contains no non-upgradeable objects before the test
    # Note: This is especially important with a 5X demo cluster which contains
    # the gphdfs role by default.
    gpupgrade generate --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT" --seed-dir "$DATA_MIGRATION_INPUT_DIR" --output-dir "$DATA_MIGRATION_OUTPUT_DIR"
    gpupgrade apply    --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT" --input-dir "$DATA_MIGRATION_OUTPUT_DIR" --phase initialize
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    run_teardowns
    gpupgrade kill-services
    unset PYTHONPATH
}

@test "pg_upgrade --check detects non-upgradeable objects" {
    local schedule="${TEST_DIR}/non_upgradeable_tests/non_upgradeable_schedule"
    local tests_to_run=${NON_UPGRADEABLE_TESTS:---schedule=$schedule}

    # Note: pg_isolation2_regress requires being run from within the isolation2 directory.
    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${TEST_DIR}/non_upgradeable_tests" \
            --outputdir="${TEST_DIR}/non_upgradeable_tests" \
            --psqldir="${GPHOME_SOURCE}/bin" \
            --port="${PGPORT}" \
            "${tests_to_run}"
    popd
}

@test "pg_upgrade upgradeable tests" {
    # Create upgradeable objects in the source cluster
    # Note: pg_isolation2_regress requires being run from within the isolation2 directory.
    local schedule="${TEST_DIR}/upgradeable_tests/source_cluster_regress/upgradeable_source_schedule"
    local tests_to_run=${UPGRADEABLE_TESTS:---schedule=$schedule}

    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${TEST_DIR}/upgradeable_tests/source_cluster_regress" \
            --outputdir="${TEST_DIR}/upgradeable_tests/source_cluster_regress" \
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
    schedule="${TEST_DIR}/upgradeable_tests/target_cluster_regress/upgradeable_target_schedule"
    tests_to_run=${UPGRADEABLE_TESTS:---schedule=$schedule}

    pushd "${ISOLATION2_PATH}"
        PGOPTIONS='-c optimizer=off' ./pg_isolation2_regress \
            --init-file=init_file_isolation2 \
            --inputdir="${TEST_DIR}/upgradeable_tests/target_cluster_regress" \
            --outputdir="${TEST_DIR}/upgradeable_tests/target_cluster_regress" \
            --use-existing \
            --psqldir="${GPHOME_TARGET}/bin" \
            --port="$(gpupgrade config show --target-port)" \
            "${tests_to_run}"
    popd
}
