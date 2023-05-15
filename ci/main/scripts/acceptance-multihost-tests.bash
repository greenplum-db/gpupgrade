#!/bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

function run_migration_scripts_and_tests() {
    time ssh cdw '
        set -eux -o pipefail

        source gpupgrade_src/ci/main/scripts/environment.bash
        source "${GPHOME_SOURCE}"/greenplum_path.sh

        echo "Running data migration scripts to ensure a clean cluster..."
        gpupgrade generate --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT"
        gpupgrade apply    --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT" --phase initialize

        ./gpupgrade_src/test/acceptance/gpupgrade/revert.bats
  '
}

main() {
    echo "Enabling ssh to cluster..."
    ./ccp_src/scripts/setup_ssh_to_cluster.sh

    echo "Installing gpupgrade_src on cdw..."
    scp -rpq gpupgrade_src gpadmin@cdw:/home/gpadmin

    echo "Installing BATS..."
    rsync --archive bats centos@cdw:
    ssh centos@cdw sudo ./bats/install.sh /usr/local

    echo "Running data migration scripts and tests..."
    run_migration_scripts_and_tests
}

main
