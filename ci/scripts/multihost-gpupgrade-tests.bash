#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# Enable ssh to CCP cluster
./ccp_src/scripts/setup_ssh_to_cluster.sh

# Install gpupgrade_src on mdw
scp -rpq gpupgrade_src gpadmin@mdw:/home/gpadmin

# Install bats on mdw
scp -rpq bats centos@mdw:~
ssh centos@mdw sudo ./bats/install.sh /usr/local

time ssh mdw '
    set -eux -o pipefail

    export GPHOME_SOURCE=/usr/local/greenplum-db-source
    export GPHOME_TARGET=/usr/local/greenplum-db-target
    source "${GPHOME_SOURCE}"/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
    export PGPORT=5432

    echo "Running data migration scripts to ensure a clean cluster..."
    gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration gpupgrade_src/data-migration-scripts
    gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true

    ./gpupgrade_src/test/acceptance/gpupgrade/revert.bats
'

echo 'multihost gpupgrade acceptance tests successful.'
