#! /bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

./ccp_src/scripts/setup_ssh_to_cluster.sh

echo "Copying extensions to the source cluster..."
scp postgis_gppkg_source/postgis*.gppkg gpadmin@mdw:/tmp/postgis_source.gppkg
scp sqldump/*.sql gpadmin@mdw:/tmp/postgis_dump.sql
scp madlib_gppkg_source/madlib*.gppkg gpadmin@mdw:/tmp/madlib_source.gppkg

echo "Installing extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
    export PGPORT=5432

    echo 'Installing PostGIS...'
    gppkg -i /tmp/postgis_source.gppkg
    /usr/local/greenplum-db-source/share/postgresql/contrib/postgis-*/postgis_manager.sh postgres install
    psql postgres -f /tmp/postgis_dump.sql
    psql -d postgres <<SQL_EOF
        -- Drop postgis views containing deprecated name datatypes
        DROP VIEW geography_columns;
        DROP VIEW raster_columns;
        DROP VIEW raster_overviews;
SQL_EOF

    echo 'Installing MADlib...'
    gppkg -i /tmp/madlib_source.gppkg
    /usr/local/greenplum-db-source/madlib/bin/madpack -p greenplum -c /postgres install
    psql -d postgres <<SQL_EOF
        DROP TABLE IF EXISTS madlib_test_type;
        CREATE TABLE madlib_test_type(id int, value madlib.svec);
        INSERT INTO madlib_test_type VALUES(1, '{1,2,3}'::float8[]::madlib.svec);
        INSERT INTO madlib_test_type VALUES(2, '{4,5,6}'::float8[]::madlib.svec);
        INSERT INTO madlib_test_type VALUES(3, '{7,8,9}'::float8[]::madlib.svec);
        CREATE VIEW madlib_test_view AS SELECT madlib.normal_quantile(0.5, 0, 1);
        CREATE VIEW madlib_test_agg AS SELECT madlib.mean(value) FROM madlib_test_type;
SQL_EOF
"

echo "Running the data migration scripts on the source cluster..."
time ssh -n mdw '
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export GPHOME_SOURCE=/usr/local/greenplum-db-source
    export PGPORT=5432

    gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration
    gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true
'
