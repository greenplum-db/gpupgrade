#! /bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
export PGPORT=5432

./ccp_src/scripts/setup_ssh_to_cluster.sh

scp sqldump/dump.sql.xz gpadmin@mdw:/tmp/

echo "Loading the SQL dump into the source cluster..."
time ssh -n gpadmin@mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    export PGOPTIONS='--client-min-messages=warning'
    unxz < /tmp/dump.sql.xz | psql -f - postgres
"

echo "Running the data migration scripts and workarounds on the source cluster..."
time ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    echo 'Running data migration script workarounds...'
    psql -d regression  <<SQL_EOF

        -- gen_alter_name_type_columns.sql cannot drop the following index because
        -- its definition uses cast to deprecated name type but evaluates to integer
        DROP INDEX onek2_u2_prtl CASCADE;
SQL_EOF

    gpupgrade-migration-sql-generator.bash $GPHOME_SOURCE $PGPORT /home/gpadmin/gpupgrade
    gpupgrade-migration-sql-executor.bash $GPHOME_SOURCE $PGPORT /home/gpadmin/gpupgrade/pre-initialize || true
"

echo "Dropping views referencing deprecated objects..."
ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    # Hardcode this view since it's the only one containing a column with type name.
    psql regression -c 'DROP VIEW IF EXISTS redundantly_named_part;'
"

echo "Dropping columns with abstime, reltime, tinterval user data types..."
columns=$(ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

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
            set -eux -o pipefail

            source /usr/local/greenplum-db-source/greenplum_path.sh

            psql regression -c 'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;'
        " || echo "Drop columns with abstime, reltime, tinterval user data types failed. Continuing..."
    fi
done

echo "Dropping gp_inject_fault extension used only for regression tests and not shipped..."
databases=$(ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    psql -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT datname
        FROM	pg_database
        WHERE	datname != 'template0';
SQL_EOF
")

echo "${databases}" | while read -r database; do
    if [[ -n "${database}" ]]; then
        ssh -n mdw "
            set -eux -o pipefail

            source /usr/local/greenplum-db-source/greenplum_path.sh

            psql -d ${database} -c 'DROP EXTENSION IF EXISTS gp_inject_fault';
        " || echo "dropping gp_inject_fault extension failed. Continuing..."
    fi
done

echo "Dropping unsupported functions..."
ssh -n mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    psql -d regression -c 'DROP FUNCTION public.myfunc(integer);
    DROP AGGREGATE public.newavg(integer);'
" || echo "Dropping unsupported functions failed. Continuing..."
