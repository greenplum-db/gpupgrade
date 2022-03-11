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
scp gptext_targz/greenplum-text*.tar.gz gpadmin@mdw:/tmp/gptext.tar.gz
scp postgis_gppkg_source/postgis*.gppkg gpadmin@mdw:/tmp/postgis_source.gppkg
scp sqldump/*.sql gpadmin@mdw:/tmp/postgis_dump.sql
scp madlib_gppkg_source/madlib*.gppkg gpadmin@mdw:/tmp/madlib_source.gppkg
scp plr_gppkg_source/plr*.gppkg gpadmin@mdw:/tmp/plr_source.gppkg

echo "Installing extensions and sample data on source cluster..."

echo 'Installing GPDB extension dependencies...'
mapfile -t hosts < cluster_env_files/hostfile_all
for host in "${hosts[@]}"; do
    ssh -n "centos@${host}" "
        set -eux -o pipefail

        sudo yum install -y R # needed for plr

        sudo mkdir /usr/local/greenplum-db-text
        sudo chown gpadmin:gpadmin /usr/local/greenplum-db-text

        sudo mkdir /usr/local/greenplum-solr
        sudo chown gpadmin:gpadmin /usr/local/greenplum-solr
    "
done

echo "Installing GPDB extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail
    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=$MASTER_DATA_DIRECTORY

    echo 'Installing gptext...'
    tar -xzvf /tmp/gptext.tar.gz -C /tmp/
    chmod +x /tmp/greenplum-text*.bin
    sed -i -r 's/GPTEXT_HOSTS\=\(localhost\)/GPTEXT_HOSTS\=\"ALLSEGHOSTS\"/' /tmp/gptext_install_config
    sed -i -r 's/ZOO_HOSTS.*/ZOO_HOSTS\=\(mdw mdw mdw\)/' /tmp/gptext_install_config

    /tmp/greenplum-text*.bin -c /tmp/gptext_install_config -d /usr/local/greenplum-db-text
    source /usr/local/greenplum-db-text/greenplum-text_path.sh
    createdb gptext_db
    gptext-installsql gptext_db
    gptext-start

    psql -v ON_ERROR_STOP=1 -d gptext_db <<SQL_EOF
        CREATE TABLE gptext_test(id INT PRIMARY KEY, content TEXT);
        INSERT INTO gptext_test VALUES (1, 'Greenplum Database balabalabala');
        INSERT INTO gptext_test VALUES (2, 'VMware Greenplum balabala');

        SELECT * FROM gptext.create_index('public', 'gptext_test', 'id', 'content');
        SELECT * FROM gptext.index(table(SELECT * FROM gptext_test), 'gptext_db.public.gptext_test');
        SELECT * FROM gptext.commit_index('gptext_db.public.gptext_test');

        CREATE VIEW gptext_test_view AS SELECT * FROM gptext.search(table(SELECT 1 SCATTER BY 1), 'gptext_db.public.gptext_test', 'greenplum', NULL);
SQL_EOF

    echo 'Installing PostGIS...'
    gppkg -i /tmp/postgis_source.gppkg
    /usr/local/greenplum-db-source/share/postgresql/contrib/postgis-*/postgis_manager.sh postgres install
    psql postgres -f /tmp/postgis_dump.sql
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        -- Drop postgis views containing deprecated name datatypes
        DROP VIEW geography_columns;
        DROP VIEW raster_columns;
        DROP VIEW raster_overviews;
SQL_EOF

    echo 'Installing MADlib...'
    gppkg -i /tmp/madlib_source.gppkg
    /usr/local/greenplum-db-source/madlib/bin/madpack -p greenplum -c /postgres install
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        DROP TABLE IF EXISTS madlib_test_type;
        CREATE TABLE madlib_test_type(id int, value madlib.svec);
        INSERT INTO madlib_test_type VALUES(1, '{1,2,3}'::float8[]::madlib.svec);
        INSERT INTO madlib_test_type VALUES(2, '{4,5,6}'::float8[]::madlib.svec);
        INSERT INTO madlib_test_type VALUES(3, '{7,8,9}'::float8[]::madlib.svec);

        CREATE VIEW madlib_test_view AS SELECT madlib.normal_quantile(0.5, 0, 1);
        CREATE VIEW madlib_test_agg AS SELECT madlib.mean(value) FROM madlib_test_type;
SQL_EOF

    echo 'Installing plr...'
    gppkg -i /tmp/plr_source.gppkg
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        CREATE EXTENSION plr;
        CREATE OR REPLACE FUNCTION r_norm(n integer, mean float8, std_dev float8) RETURNS float8[ ] AS \\\$\\\$
            x<-rnorm(n,mean,std_dev)
            return(x)
        \\\$\\\$ LANGUAGE 'plr';

        CREATE VIEW test_norm_var AS SELECT id, r_norm(10,0,1) as x FROM (SELECT generate_series(1,30::bigint) AS ID) foo;
SQL_EOF
"

install_plcontainer() {
    echo "Installing plcontainer..."

    scp plcontainer_gppkg_source/*.gppkg gpadmin@mdw:/tmp/plcontainer_source.gppkg

    time ssh -n mdw "
        set -eux -o pipefail

        source /usr/local/greenplum-db-source/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=$MASTER_DATA_DIRECTORY

        gppkg -i /tmp/plcontainer_source.gppkg

        psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
            CREATE EXTENSION plcontainer;
            CREATE FUNCTION dummyPython() RETURNS text AS \\\$\\\$
                # container: plc_python_shared
                return 'hello from Python'
            \\\$\\\$ LANGUAGE plcontainer;

            CREATE VIEW plcontainer_view AS SELECT * FROM dummyPython();
SQL_EOF
"
}

test_plcontainer "$OS_VERSION" && install_plcontainer || echo "Skipping plcontainer for centos6 since its not supported..."

install_pxf() {
    local PXF_BASE=/home/gpadmin/pxf

    echo "Installing pxf on all hosts in the source cluster..."
    echo "${GOOGLE_CREDENTIALS}" > /tmp/key.json

    # PXF SNAPSHOT builds are only available as an RPM inside a tar.gz
    if compgen -G pxf_rpm_source/pxf-gp?.el7.tar.gz &>/dev/null; then
        tar -xf pxf_rpm_source/pxf-gp?.el7.tar.gz \
            --directory pxf_rpm_source \
            --strip-components=1 \
            --wildcards '*.rpm'
    fi

    mapfile -t hosts < cluster_env_files/hostfile_all
    for host in "${hosts[@]}"; do
        scp pxf_rpm_source/*.rpm "gpadmin@${host}":/tmp/pxf_source.rpm
        scp /tmp/key.json "gpadmin@${host}":/tmp/key.json

        ssh -n "centos@${host}" "
            set -eux -o pipefail

            echo 'Installing pxf...'
            sudo rpm -ivh /tmp/pxf_source.rpm
            sudo chown -R gpadmin:gpadmin /usr/local/pxf*
        "
    done

    ssh -n mdw "
        set -eux -o pipefail

        source /usr/local/greenplum-db-source/greenplum_path.sh

        echo 'Initialize pxf...'
        export GPHOME=$GPHOME_SOURCE
        export PXF_BASE=$PXF_BASE

        /usr/local/pxf-*/bin/pxf cluster prepare
        sed -i -e 's|^# export JAVA_HOME=.*|export JAVA_HOME=/usr/lib/jvm/jre|' $PXF_BASE/conf/pxf-env.sh
        mkdir -p ${PXF_BASE}/servers/google

        cp /usr/local/pxf-*/templates/gs-site.xml ${PXF_BASE}/servers/google/
        sed -i 's|YOUR_GOOGLE_STORAGE_KEYFILE|/tmp/key.json|' ${PXF_BASE}/servers/google/gs-site.xml

        /usr/local/pxf-*/bin/pxf cluster register
        /usr/local/pxf-*/bin/pxf cluster sync
        /usr/local/pxf-*/bin/pxf cluster start

        echo 'Load PXF data...'
        psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
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

test_pxf "$OS_VERSION" && install_pxf || echo "Skipping pxf for centos6 since pxf6 for GPDB6 on centos6 is not supported..."

echo "Installing postgres native extensions and sample data on source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    echo 'Installing amcheck...'
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
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
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        \i /usr/local/greenplum-db-source/share/postgresql/contrib/dblink.sql

        CREATE TABLE foo(f1 int, f2 text, primary key (f1,f2));
        INSERT INTO foo VALUES (0,'a');
        INSERT INTO foo VALUES (1,'b');
        INSERT INTO foo VALUES (2,'c');
        CREATE VIEW dblink_test_view AS SELECT * FROM dblink('dbname=postgres', 'SELECT * FROM foo') AS t(a int, b text) WHERE t.a > 7;
SQL_EOF

    echo 'Installing hstore...'
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        \i /usr/local/greenplum-db-source/share/postgresql/contrib/hstore.sql

        CREATE TABLE hstore_test_type AS SELECT 'a=>1,a=>2'::hstore as c1;
        CREATE VIEW hstore_test_view AS SELECT c1 -> 'a' as c2 FROM hstore_test_type;
SQL_EOF

    echo 'Installing pgcrypto...'
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        CREATE EXTENSION pgcrypto;

        CREATE VIEW pgcrypto_test_view AS SELECT crypt('new password', gen_salt('md5'));
SQL_EOF

    echo 'Installing Fuzzy String Match...'
    psql -d postgres -f /usr/local/greenplum-db-source/share/postgresql/contrib/fuzzystrmatch.sql
    psql -v ON_ERROR_STOP=1 -d postgres <<SQL_EOF
        CREATE VIEW fuzzystrmatch_test_view AS SELECT soundex('a'::text);
SQL_EOF

    echo 'Installing citext...'
    echo 'Create a new db to avoid potential function overlaps with postgis to simplify the diff when comparing before and after upgrade.'
    createdb citext_db
    psql -d citext_db -f /usr/local/greenplum-db-source/share/postgresql/contrib/citext.sql
    psql -v ON_ERROR_STOP=1 -d citext_db <<SQL_EOF
        CREATE TABLE citext_test_type (
            id bigint PRIMARY KEY,
            nick CITEXT NOT NULL,
            pass TEXT   NOT NULL
        ) DISTRIBUTED BY (id);

        INSERT INTO citext_test_type VALUES (1,  'larry',  md5(random()::text) );
SQL_EOF
"

echo "Running the data migration scripts on the source cluster..."
ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    gpupgrade-migration-sql-generator.bash $GPHOME_SOURCE $PGPORT /tmp/migration
    gpupgrade-migration-sql-executor.bash $GPHOME_SOURCE $PGPORT /tmp/migration/pre-initialize || true
"
