#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/scripts/ci-helpers.bash

USE_LINK_MODE=${USE_LINK_MODE:-0}
FILTER_DIFF=${FILTER_DIFF:-0}
DIFF_FILE=${DIFF_FILE:-"icw.diff"}

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export PGPORT=5432

./ccp_src/scripts/setup_ssh_to_cluster.sh

echo "Copying extensions to the target cluster..."
scp postgis_gppkg_target/postgis*.gppkg gpadmin@mdw:/tmp/postgis_target.gppkg
scp madlib_gppkg_target/madlib*.gppkg gpadmin@mdw:/tmp/madlib_target.gppkg

if ! is_GPDB5 ${GPHOME_SOURCE}; then
    echo "Configuring GUCs before dumping the source cluster..."
    configure_gpdb_gucs ${GPHOME_SOURCE}
fi

echo "Dumping the source cluster for comparing after upgrade..."
dump_sql $PGPORT /tmp/source.sql

echo "Performing gpupgrade..."
LINK_MODE=""
if [ "${USE_LINK_MODE}" = "1" ]; then
    LINK_MODE="--mode=link"
fi

time ssh -n mdw '
    set -ex -o pipefail

    export GPHOME_SOURCE=/usr/local/greenplum-db-source
    export GPHOME_TARGET=/usr/local/greenplum-db-target
    export PGPORT=5432

    echo "Running initialize to create target cluster...."
    echo "Initialize expected to fail as target extension is not yet installed since target cluster is needed..."
    set +e
    gpupgrade initialize \
              $LINK_MODE \
              --automatic \
              --target-gphome ${GPHOME_TARGET} \
              --source-gphome ${GPHOME_SOURCE} \
              --source-master-port $PGPORT \
              --temp-port-range 6020-6040
    set -e

    echo "Installing extensions on the target cluster..."
    source /usr/local/greenplum-db-target/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=$(gpupgrade config show --target-datadir)
    export PGPORT=$(gpupgrade config show --target-port)

    gpstart -a
    gppkg -i /tmp/postgis_target.gppkg
    gppkg -i /tmp/madlib_target.gppkg
    gpstop -a

    echo "Finishing the upgrade..."
    gpupgrade initialize \
          $LINK_MODE \
          --automatic \
          --target-gphome ${GPHOME_TARGET} \
          --source-gphome ${GPHOME_SOURCE} \
          --source-master-port $PGPORT \
          --temp-port-range 6020-6040

    gpupgrade execute --non-interactive
    gpupgrade finalize --non-interactive
'

if ! is_GPDB5 ${GPHOME_TARGET}; then
    echo "Configuring GUCs before dumping the target cluster..."
    configure_gpdb_gucs ${GPHOME_TARGET}

    echo "Reindexing all databases to enable bitmap indexes which were marked invalid during the upgrade...."
    reindex_all_dbs ${GPHOME_TARGET}
fi

echo "Dumping the target cluster..."
dump_sql ${PGPORT} /tmp/target.sql

echo "Comparing the source and target dumps..."
if ! compare_dumps /tmp/source.sql /tmp/target.sql; then
    echo 'error: before and after dumps differ'
    exit 1
fi

echo "Applying post-upgrade extension fixups after comparing dumps..."
time ssh -n gpadmin@mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
    export PGPORT=5432

    echo 'Recreating dropped views that contained the deprecated name datatype...'
    psql -d postgres -f /usr/local/greenplum-db-target/share/postgresql/contrib/postgis-*/postgis_replace_views.sql

    echo 'Dropping operator dependent objects in order to successfully drop and recreate postgis operators...'
    psql -d postgres -c 'DROP INDEX wmstest_geomidx CASCADE;'
    psql -d postgres -f /usr/local/greenplum-db-target/share/postgresql/contrib/postgis-*/postgis_enable_operators.sql
"

echo 'Upgrade successful...'
