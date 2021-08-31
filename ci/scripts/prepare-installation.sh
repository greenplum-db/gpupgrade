#! /bin/sh
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# This script:
# - Installs the target GPDB rpm if different from source GPDB version.
# - Creates source and target GPHOME symlinks, to abstract the source and target
#   locations for future tasks to not need to know the exact versions being used.
# - Installs gpugprade RPM so future tasks have data migration scripts installed.
#
# Expected inputs are SOURCE_PACKAGE_NAME TARGET_PACKAGE_NAME such as
# greenplum-db-5, or greenplum-db-6.

set -eux -o pipefail

source_package=$1
target_package=$2

apk add --no-progress openssh-client

echo "Enabling ssh to the ccp cluster..."
cp -R cluster_env_files/.ssh /root/.ssh

for host in `cat cluster_env_files/hostfile_all`; do
    if [ "$source_package" != "$target_package" ]; then
        echo "Installing the target version rpm on ${host}..."
        scp rpm_gpdb_target/*.rpm "${host}:/tmp/bin_gpdb_target.rpm"
        ssh -ttn centos@"$host" sudo yum install -y /tmp/bin_gpdb_target.rpm
    fi

    echo "Creating the source and target symlinks on ${host}..."
    ssh -n centos@"$host" "
        set -eux -o pipefail

        version=\$(rpm -q --qf '%{version}' '$source_package' | tr _ -)
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-source
        sudo chown -R gpadmin:gpadmin /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-source

        version=\$(rpm -q --qf '%{version}' '$target_package' | tr _ -)
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-target
        sudo chown -R gpadmin:gpadmin /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-target
    "

    echo "Installing the gpupgrade rpm on host ${host}..."
    scp enterprise_rpm/gpupgrade-*.rpm gpadmin@$host:/tmp
    ssh centos@$host sudo rpm -ivh /tmp/gpupgrade-*.rpm
done
