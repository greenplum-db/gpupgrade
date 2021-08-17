#! /bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/scripts/ci-helpers.bash

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
export PGPORT=5432

./ccp_src/scripts/setup_ssh_to_cluster.sh

echo "Copying extensions to the source cluster..."
scp postgis_gppkg_source/postgis*.gppkg gpadmin@mdw:/tmp/postgis_source.gppkg
scp sqldump/*.sql gpadmin@mdw:/tmp/postgis_dump.sql
scp madlib_gppkg_source/madlib*.gppkg gpadmin@mdw:/tmp/madlib_source.gppkg
scp gptext_pkg_source/greenplum-text-3.7.0-rhel6_x86_64.tar.gz gpadmin@mdw:/tmp/greenplum-text-3.7.0-rhel6_x86_64.tar.gz

echo "Installing extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=$MASTER_DATA_DIRECTORY

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

echo "Installing postgres native extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    echo 'Installing amcheck...'
    psql -d postgres <<SQL_EOF
        CREATE EXTENSION amcheck;

        CREATE VIEW amcheck_test_view AS
          SELECT bt_index_check(c.oid)::TEXT, c.relpages
          FROM pg_index i
          JOIN pg_opclass op ON i.indclass[0] = op.oid
          JOIN pg_am am ON op.opcmethod = am.oid
          JOIN pg_class c ON i.indexrelid = c.oid
          JOIN pg_namespace n ON c.relnamespace = n.oid
          WHERE am.amname = 'btree' AND n.nspname = 'pg_catalog'
            -- Function may throw an error when this is omitted:
            AND i.indisready AND i.indisvalid
          ORDER BY c.relpages DESC LIMIT 10;
SQL_EOF

    echo 'Installing dblink...'
    psql -d postgres <<SQL_EOF
        \i /usr/local/greenplum-db-source/share/postgresql/contrib/dblink.sql

        CREATE TABLE foo(f1 int, f2 text, primary key (f1,f2));
        INSERT INTO foo VALUES (0,'a');
        INSERT INTO foo VALUES (1,'b');
        INSERT INTO foo VALUES (2,'c');
        CREATE VIEW dblink_test_view AS SELECT * FROM dblink('dbname=postgres', 'SELECT * FROM foo') AS t(a int, b text) WHERE t.a > 7;
SQL_EOF

    echo 'Installing hstore...'
    psql -d postgres <<SQL_EOF
        \i /usr/local/greenplum-db-source/share/postgresql/contrib/hstore.sql

        CREATE TABLE hstore_test_type AS
        SELECT 'a=>1,a=>2'::hstore as c1;
        CREATE VIEW hstore_test_view AS SELECT c1 -> 'a' as c2 FROM foo;
SQL_EOF

    echo 'Installing pgcrypto...'
    psql -d postgres <<SQL_EOF
        CREATE EXTENSION pgcrypto;

        CREATE VIEW pgcrypto_test_view AS SELECT crypt('new password', gen_salt('md5'));
SQL_EOF

    echo 'Installing orafce...'
    psql -d postgres <<SQL_EOF
        CREATE EXTENSION orafce;

        CREATE TABLE orafce_test_type (a VARCHAR2(5), b NVARCHAR2(5));
        INSERT INTO orafce_test_type VALUES ('abc'::VARCHAR2(5), 'abcdef'::NVARCHAR2(5));
        CREATE VIEW orafce_test_view AS SELECT add_months('2003-08-01', 3);
SQL_EOF
"

install_pxf() {
    local PXF_CONF=/home/gpadmin/pxf

    echo "Installing pxf on all hosts in the source cluster..."
    echo "${GOOGLE_CREDENTIALS}" > /tmp/key.json

    mapfile -t hosts < cluster_env_files/hostfile_all
    for host in "${hosts[@]}"; do
        scp pxf_rpm_source/*.rpm "gpadmin@${host}":/tmp/pxf_source.rpm
        scp /tmp/key.json "gpadmin@${host}":/tmp/key.json

        ssh -n "centos@${host}" "
            set -eux -o pipefail

            echo 'Installing pxf dependencies...'
            sudo yum install -q -y java-1.8.0-openjdk.x86_64
            sudo rpm -ivh /tmp/pxf_source.rpm
            sudo chown -R gpadmin:gpadmin /usr/local/pxf*
        "
    done

    ssh -n mdw "
        set -eux -o pipefail

        source /usr/local/greenplum-db-source/greenplum_path.sh

        echo 'Initialize pxf...'
        export GPHOME=$GPHOME_SOURCE
        export PXF_CONF=$PXF_CONF
        export JAVA_HOME=/usr/lib/jvm/jre

        mkdir -p ${PXF_CONF}/servers/google
        /usr/local/pxf-*/bin/pxf cluster init

        cp /home/gpadmin/pxf/templates/gs-site.xml ${PXF_CONF}/servers/google/
        sed -i 's|YOUR_GOOGLE_STORAGE_KEYFILE|/tmp/key.json|' ${PXF_CONF}/servers/google/gs-site.xml
        /usr/local/pxf-*/bin/pxf cluster sync
        /usr/local/pxf-*/bin/pxf cluster start

        echo 'Load PXF data...'
        psql -d postgres <<SQL_EOF
            CREATE EXTENSION pxf;

            CREATE EXTERNAL TABLE pxf_read_test (a TEXT, b TEXT, c TEXT)
                LOCATION ('pxf://tmp/dummy1'
                          '?FRAGMENTER=org.greenplum.pxf.api.examples.DemoFragmenter'
                          '&ACCESSOR=org.greenplum.pxf.api.examples.DemoAccessor'
                          '&RESOLVER=org.greenplum.pxf.api.examples.DemoTextResolver')
                FORMAT 'TEXT' (DELIMITER ',');
            CREATE TABLE pxf_read_test_materialized AS SELECT * FROM pxf_read_test;


            CREATE EXTERNAL TABLE pxf_parquet_read (id INTEGER, name TEXT, cdate DATE, amt DOUBLE PRECISION, grade TEXT,
                                                b BOOLEAN, tm TIMESTAMP WITHOUT TIME ZONE, bg BIGINT, bin BYTEA,
                                                sml SMALLINT, r REAL, vc1 CHARACTER VARYING(5), c1 CHARACTER(3),
                                                dec1 NUMERIC, dec2 NUMERIC(5,2), dec3 NUMERIC(13,5), num1 INTEGER)
                LOCATION ('pxf://gpupgrade-intermediates/extensions/pxf_parquet_types.parquet?PROFILE=gs:parquet&SERVER=google')
                FORMAT 'CUSTOM' (FORMATTER='pxfwritable_import');
            CREATE TABLE pxf_parquet_read_materialized AS SELECT * FROM pxf_parquet_read;
SQL_EOF

        /usr/local/pxf-*/bin/pxf cluster stop
"
}

test_pxf "$OS_VERSION" && install_pxf || echo "Skipping pxf for centos6 since pxf5 for GPDB6 on centos6 is not supported..."

echo "Installing gptext 3.7.0 on source cluster..."
mapfile -t hosts < cluster_env_files/hostfile_all
for host in "${hosts[@]}"; do
    ssh -n "centos@${host}" "
        sudo yum install java-1.8.0-openjdk
        sudo mkdir /usr/local/greenplum-db-text-3.7.0
        sudo mkdir /usr/local/greenplum-solr
        sudo chown gpadmin:gpadmin /usr/local/greenplum-db-text-3.7.0
        sudo chown gpadmin:gpadmin /usr/local/greenplum-solr
    "
done
time ssh -n mdw "
export MASTER_DATA_DIRECTORY=$MASTER_DATA_DIRECTORY
source /usr/local/greenplum-db-source/greenplum_path.sh

tar zxvf /tmp/greenplum-text-3.7.0-rhel6_x86_64.tar.gz -C /tmp/
chmod +x /tmp/greenplum-text-3.7.0-rhel6_x86_64.bin
sed -i -r 's/GPTEXT_HOSTS\=\(localhost\)/GPTEXT_HOSTS\=\"ALLSEGHOSTS\"/' /tmp/gptext_install_config
sed -i -r 's/ZOO_HOSTS.*/ZOO_HOSTS\=\(mdw sdw1 sdw1\)/' /tmp/gptext_install_config

/tmp/greenplum-text-3.7.0-rhel6_x86_64.bin -c /tmp/gptext_install_config -d /usr/local/greenplum-db-text-3.7.0
createdb demo
source /usr/local/greenplum-db-text-3.7.0/greenplum-text_path.sh
gptext-installsql demo
gptext-start
"

echo "Running the data migration scripts on the source cluster..."
ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    gpupgrade-migration-sql-generator.bash $GPHOME_SOURCE $PGPORT /tmp/migration
    gpupgrade-migration-sql-executor.bash $GPHOME_SOURCE $PGPORT /tmp/migration/pre-initialize || true
"
