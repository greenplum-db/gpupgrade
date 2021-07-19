#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# NOTE: All these steps need to be done in the same task since each task is run
# in its own isolated container with no shared state. Thus, installing the RPM
# needs to be done in the same task/container as running the tests.

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

install_source_GPDB_rpm_and_symlink() {
    yum install -y rpm_gpdb_source/*.rpm

    version=$(rpm -q --qf '%{version}' "$SOURCE_PACKAGE" | tr _ -)
    sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_SOURCE"

    chown -R gpadmin:gpadmin "$GPHOME_SOURCE"
}

# XXX: Setup target cluster before sourcing greenplum_path otherwise there are
# yum errors due to python issues.
# XXX: When source equals target then yum will fail when trying to re-install.
install_target_GPDB_rpm_and_symlink() {
    if [ "$SOURCE_PACKAGE" != "$TARGET_PACKAGE" ]; then
        yum install -y rpm_gpdb_target/*.rpm
    fi

    version=$(rpm -q --qf '%{version}' "$TARGET_PACKAGE" | tr _ -)
    sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_TARGET"

    chown -R gpadmin:gpadmin "$GPHOME_TARGET"
}

create_source_cluster() {
    source "$GPHOME_SOURCE"/greenplum_path.sh

    chown -R gpadmin:gpadmin gpdb_src_soruce/gpAux/gpdemo
    su gpadmin -c "make -j $(nproc) -C gpdb_src_soruce/gpAux/gpdemo create-demo-cluster"
    source gpdb_src_soruce/gpAux/gpdemo/gpdemo-env.sh
}

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

        make install
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
