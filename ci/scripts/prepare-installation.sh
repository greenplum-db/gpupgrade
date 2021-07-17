#! /bin/sh
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# This script:
# - Installs target GPDB rpm if different from source GPDB version.
# - Creates source and target GPHOME symlinks, sot that that future pipeline
# tasks don't have to know the exact versions in use.
# - Installs gpugprade RPM so future tasks have data migration scripts installed.
#
# Expected inputs are SOURCE_PACKAGE_NAME TARGET_PACKAGE_NAME

set -eux

source_package=$1
target_package=$2

apk add --no-progress openssh-client
cp -R cluster_env_files/.ssh /root/.ssh

for host in `cat cluster_env_files/hostfile_all`; do
    if [ "$source_package" != "$target_package" ]; then
        # Install the target binary.
        scp rpm_gpdb_target/*.rpm "${host}:/tmp/bin_gpdb_target.rpm"
        ssh -ttn centos@"$host" sudo yum install -y /tmp/bin_gpdb_target.rpm
    fi

    # Install source/target symlinks.
    ssh -n centos@"$host" "
        set -eux

        version=\$(rpm -q --qf '%{version}' '$source_package' | tr _ -)
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-source

        version=\$(rpm -q --qf '%{version}' '$target_package' | tr _ -)
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-target
    "

    # Install gpupgrade RPM
    scp rpm_enterprise/gpupgrade-*.rpm gpadmin@$host:/tmp
    ssh centos@$host sudo rpm -ivh /tmp/gpupgrade-*.rpm
done
