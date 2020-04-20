#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

# Install BATS
./bats/install.sh /usr/local

source gpdb_src/concourse/scripts/common.bash
mkdir -p /usr/local/greenplum-db-devel
tar -xzf bin_gpdb/*.tar.gz -C /usr/local/greenplum-db-devel

time ./gpdb_src/concourse/scripts/setup_gpadmin_user.bash "centos"
time make_cluster

time chown -R gpadmin:gpadmin go

su gpadmin -c '
    set -ex

    export TERM=linux
    export GOPATH=$PWD/go
    export PATH=$GOPATH/bin:$PATH
    source /usr/local/greenplum-db-devel/greenplum_path.sh
    source gpdb_src/gpAux/gpdemo/gpdemo-env.sh

    cd $GOPATH/src/github.com/greenplum-db/gpupgrade
    export GOFLAGS="-mod=readonly" # do not update dependencies during build

    make
    make check --keep-going

    # Note that installcheck is currently destructive.
    make install
    make installcheck
'
