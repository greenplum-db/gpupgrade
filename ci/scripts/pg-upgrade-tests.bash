#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# NOTE: All these steps need to be done in the same task since each task is run
# in its own isolated container with no shared state. Thus, installing the RPM,
# and making isolation2 needs to be done in the same task/container.

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

source gpupgrade_src/ci/scripts/ci-helpers.bash

make_pg_isolation2_regress_for_the_target_GPDB_version() {
    # setup_configure_vars and configure expect GPHOME=/usr/local/greenplum-db-devel
    # Thus, symlink the target version to /usr/local/greenplum-db-devel.
    # Alternatively, refactor common.bash to use $GPHOME. However, due to unforeseen
    # consequences and stability concerns we cannot do that.
    ln -s "$GPHOME_TARGET" /usr/local/greenplum-db-devel
    set +u
    source gpdb_src/concourse/scripts/common.bash
    setup_configure_vars
    export LDFLAGS="-L$GPHOME_TARGET/ext/python/lib $LDFLAGS"
    configure
    set -u

    source "${GPHOME_TARGET}"/greenplum_path.sh
    make -j "$(nproc)" -C gpdb_src
    make -j "$(nproc)" -C gpdb_src/src/test/isolation2 install
}

run_pg_upgrade_tests() {
    chown -R gpadmin:gpadmin gpupgrade_src
    time su gpadmin -c '
        set -eux -o pipefail

        export TERM=linux
        export ISOLATION2_PATH=$(readlink -e gpdb_src/src/test/isolation2)

        cd gpupgrade_src
        make pg-upgrade-tests
    '
}

main() {
    echo "Installing BATS..."
    ./bats/install.sh /usr/local

    echo "Installing gpupgrade rpm..."
    yum install -y enterprise_rpm/gpupgrade-*.rpm

    echo "Setting up gpadmin user..."
    mkdir -p gpdb_src
    ./gpdb_src_source/concourse/scripts/setup_gpadmin_user.bash "centos"

    echo "Installing the source GPDB rpm and symlink..."
    install_source_GPDB_rpm_and_symlink

    echo "Installing the target GPDB rpm and symlink..."
    install_target_GPDB_rpm_and_symlink

    echo "Making pg_isolation2_regress for the target GPDB version..."
    make_pg_isolation2_regress_for_the_target_GPDB_version

    echo "Creating the source demo cluster..."
    create_source_cluster

    echo "Running tests..."
    run_pg_upgrade_tests
}

main
