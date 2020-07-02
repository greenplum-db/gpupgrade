#!/usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers
load finalize_checks

setup() {
    skip_if_no_gpdb

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    if [ -n "$NEW_CLUSTER" ]; then
        delete_finalized_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi

    gpupgrade kill-services

    restore_cluster

    # reload old path and start
    source "${GPHOME_SOURCE}/greenplum_path.sh"
    gpstart -a

    # delete tablespace data added to the source cluster
    if is_GPDB5 "${GPHOME_SOURCE}"; then
        delete_tablespace_data
    fi

    # delete the state_dir, which also contains the tablespace filesystem
    cleanup_state_dir
}

upgrade_cluster() {
        LINK_MODE=$1

        setup_restore_cluster "$LINK_MODE"

        # place marker file in source master data directory
        local marker_file=source-cluster.test-marker
        local mirror_datadirs=($(query_datadirs $GPHOME_SOURCE $PGPORT "role='m'"))
        local primary_datadirs=($(query_datadirs $GPHOME_SOURCE $PGPORT "role='p'"))
        local datadirs=($(query_datadirs $GPHOME_SOURCE $PGPORT))
        for datadir in "${datadirs[@]}"; do
            touch "$datadir/${marker_file}"
        done


        # grab the original configuration before starting so we can verify the
        # target cluster ends up with the source cluster's original layout
        local old_config=$(get_segment_configuration "${GPHOME_SOURCE}")

        # set this variable before we upgrade to make sure our decision to run below
        # is based on the source cluster before upgrade perhaps changes our cluster.
        local no_mirrors
        no_mirrors=$(contents_without_mirror "${GPHOME_SOURCE}" "$(hostname)" "${PGPORT}")

        if [ "$LINK_MODE" == "--mode=link" ]; then
               # create a backup of datadirs as the mirrors will be deleted in finalize
               # and primaries pg_control file will be changed to pg_control.old to disable to old
               # cluster
               "${GPHOME_SOURCE}"/bin/gpstop -a
               for datadir in "${datadirs[@]}"; do
                   cp -r ${datadir} ${datadir}_backup
               done
               gpstart -a
        fi

        # upgrade on 6-6 does not work due to a bug in pg_upgrade
        if is_GPDB5 "$GPHOME_SOURCE"; then
            create_tablespace_with_table
        fi

        gpupgrade initialize \
            --source-gphome="$GPHOME_SOURCE" \
            --target-gphome="$GPHOME_TARGET" \
            --source-master-port="${PGPORT}" \
            --temp-port-range 6020-6040 \
            --disk-free-ratio 0 \
            $LINK_MODE \
            --verbose 3>&-

        gpupgrade execute --verbose
        gpupgrade finalize --verbose

        if is_GPDB5 "$GPHOME_SOURCE"; then
            check_tablespace_data
        fi

        NEW_CLUSTER="$MASTER_DATA_DIRECTORY"

        if [ "$LINK_MODE" == "--mode=link" ]; then
            validate_data_directories "EXISTS" "$primary_datadirs"
            validate_data_directories "NOT_EXISTS" "$mirror_datadirs"

            # restore the data directories to their archived versions to fit the
            # teardown in link mode, finalize deletes the mirrors/standby data
            # directories, so they should be restored.
            for datadir in "${datadirs[@]}"; do
                local archive=$(archive_dir "$datadir")
                rm -rf ${archive}
                mv "${datadir}"_backup "${archive}"
            done
        else
            validate_data_directories "EXISTS" "${datadirs}"
        fi

        # ensure gpperfmon configuration file has been modified to reflect new data dir location
        local gpperfmon_config_file="${MASTER_DATA_DIRECTORY}/gpperfmon/conf/gpperfmon.conf"
        grep "${MASTER_DATA_DIRECTORY}" "${gpperfmon_config_file}" || \
            fail "got gpperfmon.conf file $(cat $gpperfmon_config_file), wanted it to include ${MASTER_DATA_DIRECTORY}"

        # Check to make sure the new cluster matches the old one.
        local new_config=$(get_segment_configuration "${GPHOME_TARGET}")
        [ "$old_config" = "$new_config" ] || fail "actual config: $new_config, wanted $old_config"

        #
        # The tests below depend on the source cluster having a standby and a full set of mirrors
        #

        if [ -n "$no_mirrors" ]; then
            echo "skipping rest of this test since these content ids does not have a standby: ${no_mirrors}"
            return 0
        fi

        # TODO: Query gp_stat_replication to check if the standby is in sync.
        #   That is a more accurate representation if the standby is running and
        #   in sync, since gpstate might simply check if the process is running.
        local new_datadir=$(gpupgrade config show --target-datadir)
        local actual_standby_status=$(source "${GPHOME_TARGET}/greenplum_path.sh" && gpstate -d "${new_datadir}")
        local standby_status_line=$(get_standby_status "$actual_standby_status")
        [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"

        validate_mirrors_and_standby "${GPHOME_TARGET}" "$(hostname)" "${PGPORT}"
}

@test "in copy mode gpupgrade finalize should swap the target data directories and ports with the source cluster" {
    upgrade_cluster
}

@test "in link mode gpupgrade finalize should also delete mirror directories" {
    upgrade_cluster "--mode=link"
}

setup_state_dir() {
    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

cleanup_state_dir() {
    if [ -n "$STATE_DIR" ]; then
        rm -r "$STATE_DIR"
    fi
}

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}

validate_data_directories() {
        CHECK_EXISTS=$1
        shift
        DATADIRS=("$@")
        for datadir in "${DATADIRS[@]}"; do
            local source_datadir=$(archive_dir "$datadir")

            if [ "$CHECK_EXISTS" == "NOT_EXISTS" ] ; then
                # ensure that <mirror_datadir>_old directory for mirrors or standby does not exists
                [ ! -d "${source_datadir}/" ] || fail "expected source data directory ${source_datadir} to not exists"
            else
                [ -d "${source_datadir}/" ] || fail "expected source data directory to be located at $source_datadir"
                [ -f "${source_datadir}/${marker_file}" ] || fail "expected ${marker_file} marker file to be in source datadir: $source_datadir"
                [ -f "${source_datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $source_datadir"
            fi

            # ensure the target cluster is located where the source used to be
            [ -d "${datadir}/" ] || fail "expected target data directory to be located at $datadir"
            [ -f "${datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $datadir"
            [ ! -f "${datadir}/${marker_file}" ] || fail "unexpected ${marker_file} marker file in target datadir: $datadir"
        done
}

# This is 5X-only due to a bug in pg_upgrade for 6-6
create_tablespace_with_table() {
    # the tablespace directory will get deleted when the STATE_DIR is deleted in teardown()
    TABLESPACE_ROOT="${STATE_DIR}"/testfs
    TABLESPACE_CONFIG="${TABLESPACE_ROOT}"/fs.txt

    # create the directories required to implement our filespace
    mkdir -p "${TABLESPACE_ROOT}"/{m,{p,m}{1,2,3}}

    # create the filespace config file
    cat <<- EOF > "$TABLESPACE_CONFIG"
				filespace:batsFS
				$(hostname):1:${TABLESPACE_ROOT}/m/demoDataDir-1
				$(hostname):2:${TABLESPACE_ROOT}/p1/demoDataDir0
				$(hostname):3:${TABLESPACE_ROOT}/p2/demoDataDir1
				$(hostname):4:${TABLESPACE_ROOT}/p3/demoDataDir2
				$(hostname):5:${TABLESPACE_ROOT}/m1/demoDataDir0
				$(hostname):6:${TABLESPACE_ROOT}/m2/demoDataDir1
				$(hostname):7:${TABLESPACE_ROOT}/m3/demoDataDir2
				$(hostname):8:${TABLESPACE_ROOT}/m/standby
EOF

    (source "${GPHOME_SOURCE}"/greenplum_path.sh && gpfilespace --config "${TABLESPACE_CONFIG}")

    # create a tablespace in said filespace and a table in that tablespace
    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				CREATE TABLESPACE batsTbsp FILESPACE batsFS;
				CREATE TABLE batsTable(a int) TABLESPACE batsTbsp;
				INSERT INTO batsTable SELECT i from generate_series(1,100)i;
EOF
}

# This is 5X-only
delete_tablespace_data() {
    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				DROP TABLE IF EXISTS batsTable;
				DROP TABLESPACE IF EXISTS batsTbsp;
				DROP FILESPACE IF EXISTS batsFS;
EOF
}

check_tablespace_data() {
    local row_count
    row_count=$("$GPHOME_TARGET"/bin/psql -d postgres -Atc "SELECT COUNT(*) FROM batsTable;")
    if (( row_count != 100 )); then
        fail "failed verifying tablespaces. batsTable got $rows want 100"
    fi
}
