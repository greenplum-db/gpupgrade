#! /bin/sh
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# Usage: $0 OLD_PACKAGE_NAME NEW_PACKAGE_NAME
#
# Performs pre-upgrade cluster fixups to prepare for upgrade. This includes the
# installation of the new GPDB binary, if it is different from the old GPDB
# binary, and the creation of -old and -new symlinks so that future pipeline
# tasks don't have to know the exact versions in use.
#
# Expected pipeline inputs:
# - cluster_env_files, for SSH setup
# - rpm_gpdb_new, containing the new GPDB RPM to install IF the old and new
#   packages are different
#

set -eux

old_package=$1
new_package=$2

apk add --no-progress openssh-client
cp -R cluster_env_files/.ssh /root/.ssh

for host in `cat cluster_env_files/hostfile_all`; do
    if [ "$old_package" != "$new_package" ]; then
        # Install the new binary.
        scp rpm_gpdb_new/*.rpm "${host}:/tmp/bin_gpdb_new.rpm"
        ssh -ttn centos@"$host" sudo yum install -y /tmp/bin_gpdb_new.rpm
    fi

    # Install old/new symlinks.
    ssh -n centos@"$host" "
        set -eux

        version=\$(rpm -q --qf '%{version}' '$old_package')
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-old

        version=\$(rpm -q --qf '%{version}' '$new_package')
        sudo ln -s /usr/local/greenplum-db-\${version} /usr/local/greenplum-db-new
    "
done
