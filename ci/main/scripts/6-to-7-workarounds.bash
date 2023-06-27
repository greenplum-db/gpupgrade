#! /bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

source gpupgrade_src/ci/main/scripts/environment.bash
source gpupgrade_src/ci/main/scripts/ci-helpers.bash
./ccp_src/scripts/setup_ssh_to_cluster.sh

# FIXME: Running analyze post-upgrade fails for materialized views. For now drop all materialized views
echo "Dropping materialized views before upgrading from 6X..."
views=$(ssh -n cdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-source/greenplum_path.sh

    psql -v ON_ERROR_STOP=0 -d regression --tuples-only --no-align --field-separator ' ' <<SQL_EOF
            SELECT relname FROM pg_class WHERE relkind = 'm';
    SQL_EOF
")

echo "${views}" | while read -r view; do
    if [[ -n "${view}" ]]; then
        ssh -n cdw "
            set -eux -o pipefail

            source /usr/local/greenplum-db-source/greenplum_path.sh

            psql -v ON_ERROR_STOP=1 -d regression -c 'DROP MATERIALIZED VIEW IF EXISTS ${view}';
        " || echo "Dropping materialized views failed. Continuing..."
    fi
done
