#!/bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export PGPORT=5432

./ccp_src/scripts/setup_ssh_to_cluster.sh

# Cache our list of hosts to loop over below.
mapfile -t hosts < cluster_env_files/hostfile_all

# Copy binaries to test runner container to help compile bm.so
scp -qr mdw:${GPHOME_SOURCE} ${GPHOME_SOURCE}
scp -qr mdw:${GPHOME_TARGET} ${GPHOME_TARGET}

pushd retail_demo_src/box_muller/
  # make bm.so for source cluster
  make PG_CONFIG=${GPHOME_SOURCE}/bin/pg_config clean all

  # Install bm.so onto the segments
  for host in "${hosts[@]}"; do
      scp bm.so $host:/tmp
      ssh centos@$host "sudo mv /tmp/bm.so ${GPHOME_SOURCE}/lib/postgresql/bm.so"
  done

  # make bm.so for target cluster
  make PG_CONFIG=${GPHOME_TARGET}/bin/pg_config clean all

  # Install bm.so onto the segments for target cluster
  for host in "${hosts[@]}"; do
      scp bm.so $host:/tmp
      ssh centos@$host "sudo mv /tmp/bm.so ${GPHOME_TARGET}/lib/postgresql/bm.so"
  done
popd

# extract demo_data for both mdw and segments
pushd retail_demo_src
    tar xf demo_data.tar.xz

    pushd demo_data
        # decimate key data files to speed things up
        for f in male_first_names.txt female_first_names.txt products_full.dat surnames.dat; do
            awk 'NR % 10 == 0' "$f" > tmp.txt
            mv tmp.txt "$f"
        done
    popd
popd

# copy extracted demo_data and retail_demo_src to mdw
scp -qr retail_demo_src mdw:/home/gpadmin/industry_demo/

# create database and tables
ssh mdw <<EOF
    set -x

    source ${GPHOME_SOURCE}/greenplum_path.sh
    cd /home/gpadmin/industry_demo
    psql -d template1 -e -f data_generation/prep_database.sql
    psql -d gpdb_demo -e -f data_generation/prep_external_tables.sql
EOF

# copy extracted demo_data to segments and start gpfdist
for host in "${hosts[@]}"; do
    scp -qr retail_demo_src/demo_data/ $host:/data/

    ssh -n $host "
        set -eux -o pipefail

        source ${GPHOME_SOURCE}/greenplum_path.sh
        gpfdist -d /data/demo_data -p 8081 -l /data/demo_data/gpfdist.8081.log &
        gpfdist -d /data/demo_data -p 8082 -l /data/demo_data/gpfdist.8082.log &
    "
done

# prepare and generate data
time ssh mdw <<EOF
    set -eux -o pipefail

    source ${GPHOME_SOURCE}/greenplum_path.sh
    export PGPORT=${PGPORT}
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    # Why do we need to restart in order to have the bm.so extension take affect?
    gpstop -ar

    cd /home/gpadmin/industry_demo
    psql -d gpdb_demo -e -f data_generation/prep_UDFs.sql

    data_generation/prep_GUCs.sh

    # prepare data
    psql -d gpdb_demo -e -f data_generation/prep_retail_xts_tables.sql
    psql -d gpdb_demo -e -f data_generation/prep_dimensions.sql
    psql -d gpdb_demo -e -f data_generation/prep_facts.sql

    # generate data
    psql -d gpdb_demo -e -f data_generation/gen_order_base.sql
    psql -d gpdb_demo -e -f data_generation/gen_facts.sql
    psql -d gpdb_demo -e -f data_generation/gen_load_files.sql
    psql -d gpdb_demo -e -f data_generation/load_RFMT_Scores.sql

    # verify data
    # TODO: assert on the output of verification script
    psql -d gpdb_demo -e -f data_generation/verify_data.sql

    # XXX: This is a workaround for the following pg_upgrade check failure:
    # "ERROR: could not create relation
    # file 'base/16384/17214', relation name 'info_rels': File exists"
    gpstop -ar
EOF

# perform upgrade fixups:
ssh mdw "
    set -eux -o pipefail

    source ${GPHOME_SOURCE}/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /home/gpadmin/gpupgrade
    gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /home/gpadmin/gpupgrade/pre-initialize || true

    # match root/child partition schemas
    psql -d gpdb_demo <<SQL_EOF
        ALTER TABLE retail_demo.order_lineitems SET SCHEMA retail_parts;
        ALTER TABLE retail_demo.shipment_lineitems SET SCHEMA retail_parts;
        ALTER TABLE retail_demo.orders SET SCHEMA retail_parts;
SQL_EOF

    # XXX: This is a workaround for the following pg_upgrade check failure:
    # ERROR: could not create relation
    # file 'base/16384/17214', relation name 'info_rels': File exists
    gpcheckcat -p $PGPORT gpdb_demo
"
