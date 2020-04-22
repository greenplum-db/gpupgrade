#!/usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers
load finalize_checks

setup() {
    skip_if_no_gpdb

    PSQL="$GPHOME/bin/psql -X --no-align --tuples-only postgres"

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    if [ -n "$NEW_CLUSTER" ]; then
        delete_finalized_cluster $NEW_CLUSTER
    fi

    gpupgrade kill-services

    # reload old path and start
    source "${GPHOME}/greenplum_path.sh"
    gpstart -a

}

upgrade_cluster() {

        LINK_MODE=$1

        # place marker file in source master data directory
        local marker_file=source-cluster.test-marker
        local mirror_datadirs=($(get_mirror_datadirs))
        local primary_datadirs=($(get_primary_datadirs))
        local datadirs=($(get_datadirs))
        for datadir in "${datadirs[@]}"; do
            touch "$datadir/${marker_file}"
        done


        # grab the original configuration before starting so we can verify the
        # target cluster ends up with the source cluster's original layout
        local old_config=$(get_segment_configuration)

        # set this variable before we upgrade to make sure our decision to run below
        # is based on the source cluster before upgrade perhaps changes our cluster.
        local no_mirrors
        no_mirrors=$(contents_without_mirror "${GPHOME}" "$(hostname)" "${PGPORT}")

        if [ "$LINK_MODE" == "--link" ]; then
               # create a backup of datadirs as the mirrors will be deleted in finalize
               # and primaries pg_control file will be changed to pg_control.old to disable to old
               # cluster
               source "${GPHOME}/greenplum_path.sh"
               gpstop -a
               for datadir in "${datadirs[@]}"; do
                   cp -r ${datadir} ${datadir}_backup
               done
               gpstart -a
        fi

        gpupgrade initialize \
            --source-bindir="$GPHOME/bin" \
            --target-bindir="$GPHOME/bin" \
            --source-master-port="${PGPORT}" \
            --temp-port-range 6020-6040 \
            --disk-free-ratio 0 \
            $LINK_MODE \
            --verbose 3>&-

        gpupgrade execute --verbose
        gpupgrade finalize --verbose

        NEW_CLUSTER="$MASTER_DATA_DIRECTORY"

        if [ "$LINK_MODE" == "--link" ]; then
            validate_data_directories "EXISTS" "$primary_datadirs"
            validate_data_directories "NOT_EXISTS" "$mirror_datadirs"

            # restore the data directories to their archived versions to fit the
            # teardown in --link mode, finalize deletes the mirrors/standby data
            # directories, so they should be restored.
            for datadir in "${datadirs[@]}"; do
                local archive=$(archive_dir "$datadir")
                rm -rf ${archive}
                mv ${datadir}_backup ${archive}
            done
        else
            validate_data_directories "EXISTS" "${datadirs}"
        fi

        # ensure gpperfmon configuration file has been modified to reflect new data dir location
        local gpperfmon_config_file="${MASTER_DATA_DIRECTORY}/gpperfmon/conf/gpperfmon.conf"
        grep "${MASTER_DATA_DIRECTORY}" "${gpperfmon_config_file}" || \
            fail "got gpperfmon.conf file $(cat $gpperfmon_config_file), wanted it to include ${MASTER_DATA_DIRECTORY}"

        # Check to make sure the new cluster matches the old one.
        local new_config=$(get_segment_configuration)
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
        local actual_standby_status=$(gpstate -d "${new_datadir}")
        local standby_status_line=$(get_standby_status "$actual_standby_status")
        [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"

        validate_mirrors_and_standby "${GPHOME}" "$(hostname)" "${PGPORT}"

}
@test "gpupgrade finalize should swap the target data directories and ports with the source cluster" {
    upgrade_cluster
}

@test "gpupgrade finalize with --link mode should swap the primary and master directory and delete the old mirror and standby directory" {
    upgrade_cluster "--link"
}

setup_state_dir() {
    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

# Writes the pieces of gp_segment_configuration that we need to ensure remain
# the same across upgrade, one segment per line, sorted by content ID.
get_segment_configuration() {
    $PSQL -c "
        select content, role, hostname, port, datadir
          from gp_segment_configuration
          order by content, role
    "
}

# Writes all datadirs in the system to stdout, one per line.
get_datadirs() {
    $PSQL -Atc "select datadir from gp_segment_configuration"
}

get_primary_datadirs() {
    $PSQL -Atc "select datadir from gp_segment_configuration where role='p'"
}

get_mirror_datadirs() {
    $PSQL -Atc "select datadir from gp_segment_configuration where role='m'"
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
