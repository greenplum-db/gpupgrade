#! /bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/main/scripts/environment.bash
source gpupgrade_src/ci/main/scripts/ci-helpers.bash
./ccp_src/scripts/setup_ssh_to_cluster.sh

echo "Dropping views referencing deprecated objects..."
ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    # Hardcode this view since it's the only one containing a column with type name.
    psql -v ON_ERROR_STOP=1 regression -c 'DROP VIEW IF EXISTS redundantly_named_part;'
"

echo "Dropping columns with abstime, reltime, tinterval user data types..."
columns=$(ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    # Disable ON_ERROR_STOP due to 6X incompatibility. The
    # gp_distrbution_policy's column attrnums was renamed to distkey
    psql -v ON_ERROR_STOP=0 -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
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
        ssh -n cdw "
            set -eux -o pipefail

            source /usr/local/greenplum-db-source/greenplum_path.sh

            psql -v ON_ERROR_STOP=1 -d regression -c 'SET SEARCH_PATH TO ${schema}; ALTER TABLE ${table} DROP COLUMN ${column} CASCADE;'
        " || echo "Drop columns with abstime, reltime, tinterval user data types failed. Continuing..."
    fi
done

echo "Dropping unsupported functions..."
ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    psql -v ON_ERROR_STOP=1 -d regression -c 'DROP FUNCTION public.myfunc(integer);
    DROP AGGREGATE public.newavg(integer);'
" || echo "Dropping unsupported functions failed. Continuing..."
