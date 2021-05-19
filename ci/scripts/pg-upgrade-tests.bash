#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

# NOTE: All these steps need to be done in the same task since each task is run
# in its own isolated container with no shared state. Thus, installing the RPM,
# or making isolation2 would not be shared between tasks (and thus containers).

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

################################################################################
# Install BATS
################################################################################
./bats/install.sh /usr/local

################################################################################
# Install gpupgrade RPM
################################################################################
yum install -y rpm_enterprise/gpupgrade-*.rpm

################################################################################
# Install source and target GPDB RPMs
################################################################################
# setup gpadmin user
./gpdb_src/concourse/scripts/setup_gpadmin_user.bash "centos"

# install source packages
yum install -y rpm_gpdb_source/*.rpm

version=$(rpm -q --qf '%{version}' "$SOURCE_PACKAGE")
ln -s /usr/local/greenplum-db-${version} "$GPHOME_SOURCE"

chown -R gpadmin:gpadmin "$GPHOME_SOURCE"

# install target packages
# XXX: Setup target cluster before sourcing greenplum_path otherwise there are
# yum errors due to python issues.
# XXX: When source equals target then yum will fail when trying to re-install.
if [ "$SOURCE_PACKAGE" != "$TARGET_PACKAGE" ]; then
  yum install -y rpm_gpdb_target/*.rpm
fi

version=$(rpm -q --qf '%{version}' "$TARGET_PACKAGE")
ln -s /usr/local/greenplum-db-${version} "$GPHOME_TARGET"

chown -R gpadmin:gpadmin "$GPHOME_TARGET"

################################################################################
# Make pg_isolation2_regress for target version
################################################################################
# setup_configure_vars and configure expect GPHOME=/usr/local/greenplum-db-devel
# Thus, symlink the target version to /usr/local/greenplum-db-devel.
# Alternatively, refactor common.bash to use $GPHOME. However, due to unforeseen
# consequences and stability concerns we cannot do that.
ln -s "$GPHOME_TARGET" /usr/local/greenplum-db-devel
source gpdb_src/concourse/scripts/common.bash
setup_configure_vars
export LDFLAGS="-L$GPHOME_TARGET/ext/python/lib $LDFLAGS"
configure

source "${GPHOME_TARGET}"/greenplum_path.sh
make -j "$(nproc)" -C gpdb_src
make -j "$(nproc)" -C gpdb_src/src/test/isolation2 install

################################################################################
# Create source cluster
################################################################################
source "$GPHOME_SOURCE"/greenplum_path.sh

chown -R gpadmin:gpadmin gpdb_src_source/gpAux/gpdemo
su gpadmin -c "make -j $(nproc) -C gpdb_src_source/gpAux/gpdemo create-demo-cluster"
source gpdb_src_source/gpAux/gpdemo/gpdemo-env.sh

################################################################################
# Run tests
################################################################################
chown -R gpadmin:gpadmin gpupgrade_src

su gpadmin -c "
    set -ex

    export TERM=linux
    export ISOLATION2_PATH=$(readlink -e gpdb_src/src/test/isolation2)

    cd gpupgrade_src
    make pg-upgrade-tests
"
