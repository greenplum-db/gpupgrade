#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# Install BATS
./bats/install.sh /usr/local

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

# setup gpadmin user
# setup_gpadmin_user.bash expects gpdb_src, whereas we are using
# gpdb_src_source since this is a X-to-Y upgrade test.
mkdir gpdb_src
time ./gpdb_src_source/concourse/scripts/setup_gpadmin_user.bash "centos"

# install source packages
yum install -y rpm_gpdb_source/*.rpm

version=$(rpm -q --qf '%{version}' "$SOURCE_PACKAGE" | tr _ -)
sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_SOURCE"

chown -R gpadmin:gpadmin "$GPHOME_SOURCE"

# install target packages
# XXX: Setup target cluster before sourcing greenplum_path otherwise there are
# yum errors due to python issues.
# XXX: When source equals target then yum will fail when trying to re-install.
if [ "$SOURCE_PACKAGE" != "$TARGET_PACKAGE" ]; then
    yum install -y rpm_gpdb_target/*.rpm
fi

version=$(rpm -q --qf '%{version}' "$TARGET_PACKAGE" | tr _ -)
sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_TARGET"

chown -R gpadmin:gpadmin "$GPHOME_TARGET"

# create source cluster
chown -R gpadmin:gpadmin gpdb_src_source/gpAux/gpdemo
source "$GPHOME_SOURCE"/greenplum_path.sh

pushd gpdb_src_source/gpAux/gpdemo
    time su gpadmin -c "make create-demo-cluster"
    source gpdemo-env.sh
popd

echo 'Running data migration scripts to ensure clean cluster...'
su gpadmin gpupgrade_src/data-migration-scripts/gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration gpupgrade_src/data-migration-scripts
su gpadmin gpupgrade_src/data-migration-scripts/gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true

# setup gpupgrade
chown -R gpadmin:gpadmin gpupgrade_src

su gpadmin -c '
    set -eux -o pipefail

    export TERM=linux
    export GOFLAGS="-mod=readonly" # do not update dependencies during build

    cd gpupgrade_src
    make
    make check --keep-going

    make install
'
