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

echo 'Run data migration scripts and workarounds on source cluster...'
ssh mdw '
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export GPHOME_SOURCE=/usr/local/greenplum-db-source
    export PGPORT=5432

    echo "Running data migration script workarounds..."
    psql -d regression  <<SQL_EOF
        -- gen_alter_tsquery_to_text.sql cant alter columns with indexes, so drop them first.
        DROP INDEX bt_tsq CASCADE;
        DROP INDEX qq CASCADE;

        -- gen_alter_name_type_columns.sql cant alter inherited columns, so alter the parent.
        ALTER TABLE emp ALTER COLUMN manager TYPE VARCHAR(63);

        -- gen_alter_name_type_columns.sql cant alter columns with indexes, so drop them first.
        DROP INDEX onek_stringu1 CASCADE;
        DROP INDEX onek2_stu1_prtl CASCADE;
        DROP INDEX onek2_u2_prtl CASCADE;
SQL_EOF

    gpupgrade-migration-sql-generator.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration
    gpupgrade-migration-sql-executor.bash "$GPHOME_SOURCE" "$PGPORT" /tmp/migration/pre-initialize || true
'

echo 'Fixing remaining non-upgradeable objects...'
# this is the only view that contains a column of type name, so hardcoding for now
ssh -n mdw "
    source /usr/local/greenplum-db-source/greenplum_path.sh
    export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1

    psql regression -c 'DROP VIEW IF EXISTS redundantly_named_part;'
"

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

echo 'Dropping gp_inject_fault extension...'
# gp_inject_fault is used only for regression tests and is not shipped to customers.
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

echo "${databases}" | while read -r database; do
    if [[ -n "${database}" ]]; then
        ssh -n mdw "
            source /usr/local/greenplum-db-source/greenplum_path.sh
            export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
            psql -d ${database} -c 'DROP EXTENSION IF EXISTS gp_inject_fault';
        " || echo "dropping gp_inject_fault extension failed. Continuing..."
    fi
done

echo "Dropping unsupported functions"
ssh -n mdw "
    source /usr/local/greenplum-db-source/greenplum_path.sh
    psql -d regression -c 'DROP FUNCTION public.myfunc(integer);
    DROP AGGREGATE public.newavg(integer);'
    " || echo "Dropping unsupported functions failed. Continuing..."
