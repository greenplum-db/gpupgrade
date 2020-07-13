#!/usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers
load teardown_helpers
load tablespace_helpers
load finalize_checks

setup() {
    skip_if_no_gpdb

    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    register_teardown rm -r "$STATE_DIR"

    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    backup_source_cluster "$STATE_DIR"/backup
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    gpupgrade kill-services

    run_teardowns
}

# backup_source_cluster creates an rsync'd backup of a demo cluster and restores
# its original contents during teardown.
backup_source_cluster() {
    local backup_dir=$1

    if [[ "$MASTER_DATA_DIRECTORY" != *"/datadirs/qddir/demoDataDir-1" ]]; then
        abort "refusing to back up cluster with master '$MASTER_DATA_DIRECTORY'; demo directory layout required"
    fi

    # Don't use -p. It's important that the backup directory not exist so that
    # we know we have control over it. Also, don't assume set -e is enabled: if
    # it's not, registering an rm -rf teardown anyway could be extremely
    # dangerous.
    mkdir "$backup_dir" || return $?
    register_teardown rm -rf "$backup_dir"

    local datadir_root
    datadir_root="$(realpath "$MASTER_DATA_DIRECTORY"/../..)"

    gpstop -af
    register_teardown gpstart -a

    rsync --archive "${datadir_root:?}"/ "${backup_dir:?}"/
    register_teardown rsync --archive -I --delete "${backup_dir:?}"/ "${datadir_root:?}"/

    gpstart -a
    register_teardown stop_any_cluster
}

upgrade_cluster() {
        LINK_MODE=$1

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

        if [ "$LINK_MODE" == "--mode=link" ]; then
            validate_data_directories "EXISTS" "$primary_datadirs"
            validate_data_directories "NOT_EXISTS" "$mirror_datadirs"
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
