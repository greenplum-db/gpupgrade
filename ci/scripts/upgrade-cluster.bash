#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/scripts/ci-helpers.bash

MODE=${MODE:-"copy"}
FILTER_DIFF=${FILTER_DIFF:-0}
DIFF_FILE=${DIFF_FILE:-"icw.diff"}

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
export PGPORT=5432

./ccp_src/scripts/setup_ssh_to_cluster.sh

if ! is_GPDB5 ${GPHOME_SOURCE}; then
    echo "Configuring GUCs before dumping the source cluster..."
    configure_gpdb_gucs ${GPHOME_SOURCE}
fi

echo "Dumping the source cluster for comparing after upgrade..."
dump_sql $PGPORT /tmp/source.sql

echo "Performing gpupgrade..."
time ssh -n mdw "
    set -eux -o pipefail

    gpupgrade initialize \
              --automatic \
              --target-gphome $GPHOME_TARGET \
              --source-gphome $GPHOME_SOURCE \
              --source-master-port $PGPORT \
              --mode $MODE \
              --temp-port-range 6020-6040

    gpupgrade execute --non-interactive
    gpupgrade finalize --non-interactive

    (source ${GPHOME_TARGET}/greenplum_path.sh && ${GPHOME_TARGET}/bin/gpstart -a -d /data/gpdata/master/gpseg-1)
"

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
    echo "error: before and after dumps differ"
    exit 1
fi

# TODO: Are there additional checks to ensure the cluster was actually upgraded
# since after finalize the source and target cluster appear identical such as
# data directories getting renmaed and PGPORT. Perhaps fields in pg_controldata?

echo "Upgrade successful..."
