#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

# Install BATS
./bats/install.sh /usr/local

source gpupgrade_src/test/helpers.bash

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target

# setup gpadmin user
# setup_gpadmin_user.bash expects gpdb_src, whereas we are using
# gpdb_src_source since this is a X-to-Y upgrade test.
mkdir gpdb_src
time ./gpdb_src_source/concourse/scripts/setup_gpadmin_user.bash "centos"

# install source packages
yum install -y rpm_gpdb_source/*.rpm

version=$(rpm -q --qf '%{version}' "$SOURCE_PACKAGE")
sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_SOURCE"

chown -R gpadmin:gpadmin "$GPHOME_SOURCE"

# install target packages
# XXX: Setup target cluster before sourcing greenplum_path otherwise there are
# yum errors due to python issues.
# XXX: When source equals target then yum will fail when trying to re-install.
if [ "$SOURCE_PACKAGE" != "$TARGET_PACKAGE" ]; then
    yum install -y rpm_gpdb_target/*.rpm
fi

version=$(rpm -q --qf '%{version}' "$TARGET_PACKAGE")
sudo ln -s /usr/local/greenplum-db-${version} "$GPHOME_TARGET"

chown -R gpadmin:gpadmin "$GPHOME_TARGET"

# create source cluster
chown -R gpadmin:gpadmin gpdb_src_source/gpAux/gpdemo
source "$GPHOME_SOURCE"/greenplum_path.sh

# XXX: Need to update demo_cluster.sh to handle symlinks by using find -H.
# A PR for this should be made.
sed -i.bak -E 's/(^GPPATH=`find) (\$GPSEARCH -name .*$)/\1 -H \2/' gpdb_src_source/gpAux/gpdemo/demo_cluster.sh

pushd gpdb_src_source/gpAux/gpdemo
    time su gpadmin -c "make create-demo-cluster"
    source gpdemo-env.sh
popd

# Apply 5X fixups
if is_GPDB5 "$GPHOME_SOURCE"; then
    su gpadmin -c "
        psql -d postgres <<SQL_EOF
            CREATE OR REPLACE FUNCTION drop_gphdfs() RETURNS VOID AS \\\$\\\$
            DECLARE
              rolerow RECORD;
            BEGIN
              RAISE NOTICE 'Dropping gphdfs users...';
              FOR rolerow IN SELECT * FROM pg_catalog.pg_roles LOOP
                EXECUTE 'alter role '
                  || quote_ident(rolerow.rolname) || ' '
                  || 'NOCREATEEXTTABLE(protocol=''gphdfs'',type=''readable'')';
                EXECUTE 'alter role '
                  || quote_ident(rolerow.rolname) || ' '
                  || 'NOCREATEEXTTABLE(protocol=''gphdfs'',type=''writable'')';
                RAISE NOTICE 'dropping gphdfs from role % ...', quote_ident(rolerow.rolname);
              END LOOP;
            END;
            \\\$\\\$ LANGUAGE plpgsql;

            SELECT drop_gphdfs();

            DROP FUNCTION drop_gphdfs();
SQL_EOF
    "
fi

# setup gpupgrade
chown -R gpadmin:gpadmin gpupgrade_src

su gpadmin -c '
    set -ex

    export TERM=linux
    export GOFLAGS="-mod=readonly" # do not update dependencies during build

    cd gpupgrade_src
    make
    make check --keep-going

    # Note that installcheck is currently destructive.
    make install
    make installcheck
'
