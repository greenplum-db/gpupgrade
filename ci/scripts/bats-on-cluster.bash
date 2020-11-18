#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

is_GPDB5() {
    local gphome=$1
    version=$(ssh mdw "$gphome"/bin/postgres --gp-version)
    [[ $version =~ ^"postgres (Greenplum Database) 5." ]]
}

drop_gphdfs_roles() {
  echo 'Dropping gphdfs role...'
  ssh mdw "
      set -x

      source /usr/local/greenplum-db-source/greenplum_path.sh
      export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

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
}

#
# MAIN
#

# TODO: combine this or at least pull out common functions with upgrade-cluster.bash?

# This port is selected by our CI pipeline
MASTER_PORT=5432

# We'll need this to transfer our built binaries over to the cluster hosts.
./ccp_src/scripts/setup_ssh_to_cluster.sh

# Cache our list of hosts to loop over below.
mapfile -t hosts < cluster_env_files/hostfile_all

# Install rpm onto the cluster machines.
# TODO: how to add gpupgrade in rpm installed location onto PATH?
for host in "${hosts[@]}"; do
    scp rpm_enterprise/greenplum-upgrade*.rpm "gpadmin@$host:/tmp"
    ssh centos@$host "sudo rpm -ivh /tmp/greenplum-upgrade*.rpm"
    ssh centos@$host "sudo cp /usr/local/greenplum-upgrade/gpupgrade /usr/local/bin"
done

# Install gpupgrade_src on mdw
scp -rpq gpupgrade_src gpadmin@mdw:/home/gpadmin

# Install bats on mdw
scp -rpq bats centos@mdw:~
ssh centos@mdw sudo ./bats/install.sh /usr/local

if is_GPDB5 /usr/local/greenplum-db-source; then
  drop_gphdfs_roles
fi

time ssh mdw bash <<EOF
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export GPHOME_TARGET=/usr/local/greenplum-db-target
    export PGPORT="$MASTER_PORT"
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    ./gpupgrade_src/test/revert.bats
EOF

echo 'bats test successful.'
