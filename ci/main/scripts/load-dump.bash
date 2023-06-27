#! /bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/main/scripts/environment.bash
source gpupgrade_src/ci/main/scripts/ci-helpers.bash
./ccp_src/scripts/setup_ssh_to_cluster.sh

scp sqldump/dump.sql.xz gpadmin@cdw:/tmp/

echo "Loading the SQL dump into the source cluster..."
time ssh -n gpadmin@cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh
    # This is failing due to a number of errors. Disabling ON_ERROR_STOP until this is fixed.
    unxz --threads $(nproc) /tmp/dump.sql.xz
    PGOPTIONS='--client-min-messages=warning' psql -v ON_ERROR_STOP=0 --quiet --dbname postgres -f /tmp/dump.sql
"

echo "Running the data migration scripts and workarounds on the source cluster..."
time ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    echo 'Running data migration script workarounds...'
    psql -v ON_ERROR_STOP=1 -d regression  <<SQL_EOF

        -- gen_alter_name_type_columns.sql cannot drop the following index because
        -- its definition uses cast to deprecated name type but evaluates to integer
        DROP INDEX onek2_u2_prtl CASCADE;
SQL_EOF

    gpupgrade generate --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT" --output-dir /home/gpadmin/gpupgrade
    gpupgrade apply    --non-interactive --gphome "$GPHOME_SOURCE" --port "$PGPORT" --input-dir /home/gpadmin/gpupgrade --phase initialize
"

echo "Dropping gp_inject_fault extension used only for regression tests and not shipped..."
databases=$(ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    psql -v ON_ERROR_STOP=1 -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
        SELECT datname
        FROM	pg_database
        WHERE	datname != 'template0';
SQL_EOF
")

echo "${databases}" | while read -r database; do
    if [[ -n "${database}" ]]; then
        ssh -n cdw "
            set -eux -o pipefail

            source /usr/local/greenplum-db-source/greenplum_path.sh

            psql -v ON_ERROR_STOP=1 -d ${database} -c 'DROP EXTENSION IF EXISTS gp_inject_fault';
        " || echo "dropping gp_inject_fault extension failed. Continuing..."
    fi
done

if is_GPDB5 ${GPHOME_SOURCE}; then
    echo "Applying 5-to-6 workarounds..."
    bash gpupgrade_src/ci/main/scripts/5-to-6-workarounds.bash
elif is_GPDB6 ${GPHOME_SOURCE}; then
    echo "Applying 6-to-7 workarounds..."
    bash gpupgrade_src/ci/main/scripts/6-to-7-workarounds.bash
fi
