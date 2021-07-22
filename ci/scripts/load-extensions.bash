#! /bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

./ccp_src/scripts/setup_ssh_to_cluster.sh

echo "Copying extensions to the source cluster..."
scp postgis_gppkg_source/postgis*.gppkg gpadmin@mdw:/tmp/postgis_source.gppkg
scp sqldump/*.sql gpadmin@mdw:/tmp/postgis_dump.sql

echo "Installing extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
    export PGPORT=5432

    gppkg -i /tmp/postgis_source.gppkg
    /usr/local/greenplum-db-source/share/postgresql/contrib/postgis-*/postgis_manager.sh postgres install

    psql postgres -f /tmp/postgis_dump.sql
"

echo "Running the data migration scripts and workarounds on the source cluster..."
time ssh -n mdw '
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export GPHOME_SOURCE=/usr/local/greenplum-db-source
    export PGPORT=5432

    echo "Running data migration script workarounds..."
    psql -d postgres <<SQL_EOF
        -- Drop postgis views containing deprecated name datatypes
        DROP VIEW geography_columns;
        DROP VIEW raster_columns;
        DROP VIEW raster_overviews;
SQL_EOF

    gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration
    gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true
'
