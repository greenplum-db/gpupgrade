#!/usr/bin/env bats
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    # If this variable is set (to a coordinator data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    $PSQL -v ON_ERROR_STOP=1 -d postgres -c "drop table if exists test_linking;"

    gpupgrade kill-services
    archive_state_dir "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi

    start_source_cluster
}

@test "gpupgrade execute step to upgrade coordinator should always rsync the coordinator data dir from backup" {
    require_gnu_stat
    setup_restore_cluster "--mode=link"

    delete_target_datadirs "${MASTER_DATA_DIRECTORY}"

    gpupgrade initialize \
        --non-interactive \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --mode="link" \
        --disk-free-ratio 0 \
        --verbose

    local datadir
    datadir="$(gpupgrade config show --target-datadir)"
    NEW_CLUSTER="${datadir}"

    # Initialize creates a backup of the target coordinator data dir, during execute
    # upgrade coordinator steps refreshes the content of the target coordinator data dir
    # with the existing backup. Remove the target coordinator data directory to
    # ensure that initialize created a backup and upgrade coordinator refreshed the
    # target coordinator data directory with the backup.
    abort_unless_target_coordinator "${datadir}"
    rm -rf "${datadir:?}"/*

    # create an extra file to ensure that its deleted during rsync as we pass
    # --delete flag
    mkdir "${datadir}"/base_extra
    touch "${datadir}"/base_extra/1101
    gpupgrade execute --non-interactive --verbose
    
    # check that the extraneous files are deleted
    [ ! -d "${datadir}"/base_extra ]

    restore_cluster
}

# TODO: this test is a replica of one in initialize.bats. If/when we start to
# make a third copy for finalize, decide whether the implementations should be
# shared via helpers, or consolidated into one file or test, or otherwise --
# depending on what makes the most sense at that time.
@test "all substeps can be re-run after completion" {
    setup_restore_cluster "--mode=copy"

    gpupgrade initialize \
        --non-interactive \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    gpupgrade execute --non-interactive --verbose 3>&-

    # On GPDB5, restore the primary and coordinator directories before starting the cluster
    restore_cluster

    # Put the source and target clusters back the way they were.
    # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    (unset LD_LIBRARY_PATH; source "$GPHOME_TARGET"/greenplum_path.sh && gpstop -a -d "$NEW_CLUSTER")
    start_source_cluster

    # Mark every substep in the status file as failed. Then re-execute.
    sed -i.bak -e 's/"complete"/"failed"/g' "$GPUPGRADE_HOME/substeps.json"

    gpupgrade execute --non-interactive --verbose 3>&-

    restore_cluster
}
