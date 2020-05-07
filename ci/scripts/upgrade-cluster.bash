#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

dump_sql() {
    local port=$1
    local dumpfile=$2

    echo "Dumping cluster contents from port ${port} to ${dumpfile}..."

    ssh -n mdw "
        source ${GPHOME_NEW}/greenplum_path.sh
        pg_dumpall -p ${port} -f '$dumpfile'
    "
}

compare_dumps() {
    local old_dump=$1
    local new_dump=$2

    echo "Comparing dumps at ${old_dump} and ${new_dump}..."

    pushd gpupgrade_src
        # 5 to 6 requires some massaging of the diff due to expected changes.
        if (( $FILTER_DIFF )); then
            go build ./ci/scripts/filter
            scp ./filter mdw:/tmp/filter

            # First filter out any algorithmically-fixable differences, then
            # patch out the remaining expected diffs explicitly.
            ssh mdw "
                /tmp/filter < '$new_dump' > '$new_dump.filtered'
                patch -R '$new_dump.filtered'
            " < ./ci/scripts/filter/${DIFF_FILE}

            new_dump="$new_dump.filtered"
        fi
    popd

    ssh -n mdw "
        diff -U3 --speed-large-files --ignore-space-change --ignore-blank-lines '$old_dump' '$new_dump'
    "
}

#
# MAIN
#

# Global parameters (default to off)
USE_LINK_MODE=${USE_LINK_MODE:-0}
FILTER_DIFF=${FILTER_DIFF:-0}
DIFF_FILE=${DIFF_FILE:-"icw.diff"}
COMPARE_DIFF=${COMPARE_DIFF:-0}

# This port is selected by our CI pipeline
MASTER_PORT=5432

# We'll need this to transfer our built binaries over to the cluster hosts.
./ccp_src/scripts/setup_ssh_to_cluster.sh

# Cache our list of hosts to loop over below.
mapfile -t hosts < cluster_env_files/hostfile_all

export GPHOME_OLD=/usr/local/greenplum-db-old
export GPHOME_NEW=/usr/local/greenplum-db-new

# Install gpupgrade binary onto the cluster machines.
chmod +x bin_gpupgrade/gpupgrade
for host in "${hosts[@]}"; do
    scp bin_gpupgrade/gpupgrade "gpadmin@$host:/tmp"
    ssh centos@$host "sudo mv /tmp/gpupgrade /usr/local/bin"
done

# Dump the old cluster for later comparison.
dump_sql $MASTER_PORT /tmp/old.sql

# Now do the upgrade.
LINK_MODE=""
if [ "${USE_LINK_MODE}" = "1" ]; then
    LINK_MODE="--mode=link"
fi

time ssh mdw bash <<EOF
    set -eux -o pipefail

    gpupgrade initialize \
              $LINK_MODE \
              --target-bindir ${GPHOME_NEW}/bin \
              --source-bindir ${GPHOME_OLD}/bin \
              --source-master-port $MASTER_PORT \
              --temp-port-range 6020-6040
    # TODO: rather than setting a temp port range, consider carving out an
    # ip_local_reserved_ports range during/after CCP provisioning.

    gpupgrade execute
    gpupgrade finalize
EOF

# TODO: how do we know the cluster upgraded?  5 to 6 is a version check; 6 to 6 ?????
#   currently, it's sleight of hand...old is on port $MASTER_PORT then new is!!!!
#   perhaps use the controldata("pg_controldata $MASTER_DATA_DIR") system identifier?

# Dump the new cluster and compare.
if (( $COMPARE_DIFF )); then
    dump_sql ${MASTER_PORT} /tmp/new.sql
    if ! compare_dumps /tmp/old.sql /tmp/new.sql; then
        echo 'error: before and after dumps differ'
        exit 1
    fi
fi

echo 'Upgrade successful.'
