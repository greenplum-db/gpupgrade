#!/bin/bash

set -ex

# Install BATS
./bats/install.sh /usr/local

# make depend fails if run as gpadmin with a dep ensure git-remote-https signal 11 failure
GOPATH="$PWD/go" PATH="$PWD/go/bin:$PATH" make -C go/src/github.com/greenplum-db/gpupgrade depend

source gpdb_src/concourse/scripts/common.bash
mkdir -p /usr/local/greenplum-db-devel
tar -xzf bin_gpdb/*.tar.gz -C /usr/local/greenplum-db-devel

time ./gpdb_src/concourse/scripts/setup_gpadmin_user.bash "centos"
time make_cluster

time chown -R gpadmin:gpadmin go

su gpadmin <<'EOF'
set -ex

export GOPATH=$PWD/go
export PATH=$GOPATH/bin:$PATH
source /usr/local/greenplum-db-devel/greenplum_path.sh
source gpdb_src/gpAux/gpdemo/gpdemo-env.sh

cd $GOPATH/src/github.com/greenplum-db/gpupgrade
make
make check --keep-going

# Note that installcheck is currently destructive.
make install
make installcheck
EOF

