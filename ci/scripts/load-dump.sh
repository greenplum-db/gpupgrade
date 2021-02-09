#! /bin/sh
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

apk add --no-progress openssh-client
cp -R cluster_env_files/.ssh /root/.ssh

scp sqldump/dump.sql.xz gpadmin@mdw:/tmp/

echo 'Loading SQL dump into source cluster...'
time ssh -n gpadmin@mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export PGOPTIONS='--client-min-messages=warning'
    unxz < /tmp/dump.sql.xz | psql -f - postgres
"

echo 'Dropping gphdfs role...'
ssh mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression <<SQL_EOF
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

echo 'Dropping unique and primary keys on partitioned tables...'
tables_keys=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT rel.relname table_name, conname constraint_name
                FROM pg_constraint con
                    JOIN pg_depend dep ON (refclassid, classid, objsubid) =
                                            ('pg_constraint'::regclass, 'pg_class'::regclass, 0)
                    AND refobjid = con.oid AND deptype = 'i' AND
                                            contype IN ('u', 'p')
                    JOIN pg_class c ON dep.objid = c.oid AND relkind = 'i'
                    JOIN pg_class rel on (con.conrelid = rel.oid)
                WHERE conname <> c.relname AND c.relhassubclass='t';
SQL_EOF
")

if [ -n "${tables_keys}" ]; then
    echo "${tables_keys}" | while read -r table key; do
        ssh -n mdw "
        source /usr/local/greenplum-db-source/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

        psql regression -c 'ALTER TABLE ${table} DROP CONSTRAINT ${key} CASCADE;'
    "
    done
fi

echo 'Dropping unique and primary keys on non partitioned tables...'
tables_keys=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT rel.relname table_name, conname constraint_name
                FROM pg_constraint con
                    JOIN pg_depend dep ON (refclassid, classid, objsubid) =
                                            ('pg_constraint'::regclass, 'pg_class'::regclass, 0)
                    AND refobjid = con.oid AND deptype = 'i' AND
                                            contype IN ('u', 'p')
                    JOIN pg_class c ON dep.objid = c.oid AND relkind = 'i'
                    JOIN pg_class rel on (con.conrelid = rel.oid)
                WHERE conname <> c.relname AND c.relhassubclass='f';
SQL_EOF
")

if [ -n "${tables_keys}" ]; then
    echo "${tables_keys}" | while read -r table key; do
        ssh -n mdw "
        source /usr/local/greenplum-db-source/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

        psql regression -c 'ALTER TABLE ${table} DROP CONSTRAINT ${key} CASCADE;'
    "
    done
fi

echo 'Dropping columns with name types...'
columns=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT n.nspname, c.relname, a.attname
        FROM	pg_catalog.pg_class c,
            pg_catalog.pg_namespace n,
            pg_catalog.pg_attribute a
        WHERE	c.oid = a.attrelid AND
            a.attnum > 1 AND
            NOT a.attisdropped AND
            a.atttypid = 'pg_catalog.name'::pg_catalog.regtype AND
            c.relnamespace = n.oid AND
            n.nspname !~ '^pg_temp_' AND
            n.nspname !~ '^pg_toast_temp_' AND
            n.nspname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit');
SQL_EOF
")

# todo: we don't need to drop name type columns for the 6X ICW dump
echo "${columns}" | while read -r schema table column; do
    if [ -n "${column}" ]; then
        ssh -n mdw "
            source /usr/local/greenplum-db-source/greenplum_path.sh
            export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

            psql regression -c 'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;'
        " || echo "'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;' failed. Continuing..."
    fi
done

# this is the only view that contains a column of type name, so hardcoding for now
ssh -n mdw "
    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql regression -c 'DROP VIEW IF EXISTS redundantly_named_part;'
"

echo 'Dropping columns with tsquery types...'
columns=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT n.nspname, c.relname, a.attname
        FROM	pg_catalog.pg_class c,
                pg_catalog.pg_namespace n,
                pg_catalog.pg_attribute a
        WHERE	c.relkind = 'r' AND
                c.oid = a.attrelid AND
                NOT a.attisdropped AND
                a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype AND
                c.relnamespace = n.oid AND
                n.nspname !~ '^pg_temp_' AND
                n.nspname !~ '^pg_toast_temp_' AND
                n.nspname NOT IN ('pg_catalog', 'information_schema');
SQL_EOF
")

# todo: deduplicate. Appending to arrays not supported in `sh`
echo "${columns}" | while read -r schema table column; do
    if [ -n "${column}" ]; then
        ssh -n mdw "
            source /usr/local/greenplum-db-source/greenplum_path.sh
            export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

            psql regression -c 'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;'
        " || echo "'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;' failed. Continuing..."
    fi
done


echo 'Dropping columns with abstime, reltime, tinterval user data types...'
columns=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT nspname, relname, attname
        FROM   pg_catalog.pg_class c,
            pg_catalog.pg_namespace n,
            pg_catalog.pg_attribute a,
            gp_distribution_policy p
        WHERE  c.oid = a.attrelid AND
            c.oid = p.localoid AND
            a.atttypid in ('pg_catalog.abstime'::regtype,
                           'pg_catalog.reltime'::regtype,
                           'pg_catalog.tinterval'::regtype,
                           'pg_catalog.money'::regtype,
                           'pg_catalog.anyarray'::regtype) AND
            attnum = any (p.attrnums) AND
            c.relnamespace = n.oid AND
            n.nspname !~ '^pg_temp_';
SQL_EOF
")

echo "${columns}" | while read -r schema table column; do
    if [ -n "${column}" ]; then
        ssh -n mdw "
            source /usr/local/greenplum-db-source/greenplum_path.sh
            export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

            psql regression -c 'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;'
        " || echo "Drop columns with abstime, reltime, tinterval user data types failed. Continuing..."
    fi
done

echo 'Dropping extensions...'
databases=$(ssh -n mdw "
    set -x

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT datname
        FROM	pg_database
        WHERE	datname != 'template0';
SQL_EOF
")

# drop gp_inject_fault extension for all the databases
echo "${databases}" | while read -r database; do
    if [[ -n "${database}" ]]; then
        ssh -n mdw "
            source /usr/local/greenplum-db-source/greenplum_path.sh
            export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
            psql -d ${database} -c 'DROP EXTENSION IF EXISTS gp_inject_fault';
        " || echo "drop extensions failed. Continuing..."
    fi
done

echo "Dropping unsupported functions"
ssh -n mdw "
    source /usr/local/greenplum-db-source/greenplum_path.sh
    psql -d regression -c 'DROP FUNCTION public.myfunc(integer);
    DROP AGGREGATE public.newavg(integer);'
    " || echo "Dropping unsupported functions failed. Continuing..."
