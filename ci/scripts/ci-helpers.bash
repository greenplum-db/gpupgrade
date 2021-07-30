#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

is_GPDB5() {
    local gphome=$1
    version=$(ssh mdw "$gphome"/bin/postgres --gp-version)
    [[ $version =~ ^"postgres (Greenplum Database) 5." ]]
}

# set the database gucs
# 1. bytea_output: by default for bytea the output format is hex on GPDB 6,
#    so change it to escape to match GPDB 5 representation
configure_gpdb_gucs() {
    local gphome=$1
    ssh -n mdw "
        set -eux -o pipefail

        source ${gphome}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
        gpconfig -c bytea_output -v escape
        gpstop -u
"
}

reindex_all_dbs() {
    local gphome=$1
    ssh -n mdw "
        set -eux -o pipefail

        source ${gphome}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=/data/gpdata/master/gpseg-1
        reindexdb -a
"
}

dump_sql() {
    local port=$1
    local dumpfile=$2

    echo "Dumping cluster contents from port ${port} to ${dumpfile}..."

    ssh -n mdw "
        set -eux -o pipefail

        source ${GPHOME_TARGET}/greenplum_path.sh
        pg_dumpall -p ${port} -f '$dumpfile'
    "
}

compare_dumps() {
    local source_dump=$1
    local target_dump=$2

    echo "Comparing dumps at ${source_dump} and ${target_dump}..."

    pushd gpupgrade_src
        # 5 to 6 requires some massaging of the diff due to expected changes.
        if (( $FILTER_DIFF )); then
            go build ./ci/scripts/filters/filter
            scp ./filter mdw:/tmp/filter

            # First filter out any algorithmically-fixable differences, then
            # patch out the remaining expected diffs explicitly.
            ssh mdw "
                /tmp/filter -version=6 -inputFile='$target_dump' > '$target_dump.filtered'
                patch -R '$target_dump.filtered'
            " < ./ci/scripts/filters/${DIFF_FILE}

            target_dump="$target_dump.filtered"

            # Run the filter on the source dump
            ssh -n mdw "
                /tmp/filter -version=5 -inputFile='$source_dump' > '$source_dump.filtered'
            "

            source_dump="$source_dump.filtered"
        fi
    popd

    ssh -n mdw "
        diff -U3 --speed-large-files --ignore-space-change --ignore-blank-lines '$source_dump' '$target_dump'
    "
}

install_source_GPDB_rpm_and_symlink() {
    yum install -y rpm_gpdb_source/*.rpm

    version=$(rpm -q --qf '%{version}' "$SOURCE_PACKAGE" | tr _ -)
    ln -s /usr/local/greenplum-db-${version} "$GPHOME_SOURCE"

    chown -R gpadmin:gpadmin "$GPHOME_SOURCE"
}

# XXX: Setup target cluster before sourcing greenplum_path otherwise there are
# yum errors due to python issues.
# XXX: When source equals target then yum will fail when trying to re-install.
install_target_GPDB_rpm_and_symlink() {
    if [ "$SOURCE_PACKAGE" != "$TARGET_PACKAGE" ]; then
        yum install -y rpm_gpdb_target/*.rpm
    fi

    version=$(rpm -q --qf '%{version}' "$TARGET_PACKAGE" | tr _ -)
    ln -s /usr/local/greenplum-db-${version} "$GPHOME_TARGET"

    chown -R gpadmin:gpadmin "$GPHOME_TARGET"
}

create_source_cluster() {
    source "$GPHOME_SOURCE"/greenplum_path.sh

    chown -R gpadmin:gpadmin gpdb_src_source/gpAux/gpdemo
    su gpadmin -c "make -j $(nproc) -C gpdb_src_source/gpAux/gpdemo create-demo-cluster"
    source gpdb_src_source/gpAux/gpdemo/gpdemo-env.sh
}
