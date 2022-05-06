#!/bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# NOTE: All these steps need to be done in the same task since each task is run
# in its own isolated container with no shared state. Thus, installing the RPM
# needs to be done in the same task/container as running the tests.

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

source gpupgrade_src/ci/scripts/ci-helpers.bash

function run_migration_scripts_and_tests() {
    chown -R gpadmin:gpadmin gpupgrade_src
    su gpadmin -c '
        set -eux -o pipefail

        export TERM=linux
        export GOFLAGS="-mod=readonly" # do not update dependencies during build

        cd gpupgrade_src
        data-migration-scripts/gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration data-migration-scripts
        data-migration-scripts/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true

        make
        make check --keep-going
    '
}

main() {
    echo "Installing BATS..."
    ./bats/install.sh /usr/local

    echo "Setting up gpadmin user..."
    mkdir -p gpdb_src
    ./gpdb_src_source/concourse/scripts/setup_gpadmin_user.bash "centos"

    echo "Installing the source GPDB rpm and symlink..."
    install_source_GPDB_rpm_and_symlink

    echo "Installing the target GPDB rpm and symlink..."
    install_target_GPDB_rpm_and_symlink

    echo "Creating the source demo cluster..."
    create_source_cluster

    echo "Running data migration scripts and tests..."
    run_migration_scripts_and_tests
}

main
