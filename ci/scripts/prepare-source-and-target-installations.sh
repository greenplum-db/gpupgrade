#! /bin/sh
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# Usage: $0 SOURCE_PACKAGE_NAME TARGET_PACKAGE_NAME
#
# Performs pre-upgrade cluster fixups to prepare for upgrade. This includes the
# installation of the target GPDB binary, if it is different from the source GPDB
# binary, and the creation of -source and -target symlinks so that future pipeline
# tasks don't have to know the exact versions in use.
#
# Expected pipeline inputs:
# - cluster_env_files, for SSH setup
# - rpm_gpdb_target, containing the target GPDB RPM to install IF the source and target
#   packages are different
#

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

        version=\$(rpm -q --qf '%{version}' '$source_package')
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-source

        version=\$(rpm -q --qf '%{version}' '$target_package')
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-target
    "
done
