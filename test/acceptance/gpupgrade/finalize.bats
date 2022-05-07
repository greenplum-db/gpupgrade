#!/usr/bin/env bats
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers
load ../helpers/teardown_helpers
load ../helpers/tablespace_helpers
load ../helpers/finalize_checks

setup() {
    skip_if_no_gpdb

    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    register_teardown archive_state_dir "$STATE_DIR"

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

upgrade_cluster() {
        MODE=$1
        HBA_HOSTNAMES=$2

        # place marker file in source coordinator data directory
        local marker_file=source-cluster.test-marker
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
        local tablespace_dirs
        if is_GPDB5 "$GPHOME_SOURCE"; then
            create_tablespace_with_tables
            tablespace_dirs=($(query_tablespace_dirs $GPHOME_SOURCE $PGPORT))
        fi

        gpupgrade initialize \
            --automatic \
            --source-gphome="$GPHOME_SOURCE" \
            --target-gphome="$GPHOME_TARGET" \
            --source-master-port="${PGPORT}" \
            --temp-port-range 6020-6040 \
            --disk-free-ratio 0 \
            --mode "$MODE" \
            "$HBA_HOSTNAMES" \
            --verbose 3>&-
        gpupgrade execute --non-interactive --verbose

        # do before gpupgrade finalize shuts down the hub
        local upgradeID
        upgradeID=$(gpupgrade config show --id)

        gpupgrade finalize --non-interactive --verbose

        # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
        (unset LD_LIBRARY_PATH; source "${GPHOME_TARGET}"/greenplum_path.sh && "${GPHOME_TARGET}"/bin/gpstart -a)

        if is_GPDB5 "$GPHOME_SOURCE"; then
            check_tablespace_data
        fi

        validate_data_directories "$upgradeID" "${datadirs}"
        is_GPDB5 "$GPHOME_SOURCE" && validate_tablespace_dirs "$tablespace_dirs"

        # ensure gpperfmon configuration file has been modified to reflect new data dir location
        local gpperfmon_config_file="${MASTER_DATA_DIRECTORY}/gpperfmon/conf/gpperfmon.conf"
        grep "${MASTER_DATA_DIRECTORY}" "${gpperfmon_config_file}" || \
            fail "got gpperfmon.conf file $(cat $gpperfmon_config_file), wanted it to include ${MASTER_DATA_DIRECTORY}"

        # Check to make sure the new cluster matches the old one.
        local new_config=$(get_segment_configuration "${GPHOME_TARGET}")
        [ "$old_config" = "$new_config" ] || fail "actual config: $new_config, wanted $old_config"

        # make sure the hub/agents are down
        ! process_is_running "[g]pupgrade hub" || fail 'expected hub to have been stopped'
        ! process_is_running "[g]pupgrade agent" || fail 'expected agent to have been stopped'

        # the GPUPGRADE_HOME directory is deleted
        [ ! -d "${GPUPGRADE_HOME}" ] || fail "expected GPUPGRADE_HOME directory ${GPUPGRADE_HOME} to have been deleted"

        # check that the archived log directory corresponds to this tests upgradeID
        if [[ -z $(find "${HOME}/gpAdminLogs/gpupgrade-${upgradeID}-"* -type d) ]]; then
            fail "expected the log directory to be archived and match ${HOME}/gpAdminLogs/gpupgrade-*"
        fi

        validate_pg_hba_conf "$HBA_HOSTNAMES"

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
        # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
        local actual_standby_status=$(unset LD_LIBRARY_PATH; source "${GPHOME_TARGET}/greenplum_path.sh" && gpstate -d "${MASTER_DATA_DIRECTORY}")
        local standby_status_line=$(get_standby_status "$actual_standby_status")
        [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"

        validate_mirrors_and_standby "${GPHOME_TARGET}" "$(hostname)" "${PGPORT}"
}

# NOTE: To reduce overall test time we test --use-hba-hostnames in link mode, and without in copy mode.

@test "in copy mode gpupgrade finalize should swap the target data directories and ports with the source cluster" {
    upgrade_cluster "copy"
}

@test "in link mode gpupgrade finalize should also delete mirror directories and honors --use-hba-hostnames" {
    upgrade_cluster "link" "--use-hba-hostnames"
}

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}

validate_data_directories() {
        UPGRADE_ID=$1
        shift
        DATADIRS=("$@")
        for datadir in "${DATADIRS[@]}"; do
            local source_datadir
            source_datadir=$(archive_dir "$datadir" "$UPGRADE_ID")
            [ -d "${source_datadir}/" ] || fail "expected source data directory to be located at $source_datadir"
            [ -f "${source_datadir}/${marker_file}" ] || fail "expected ${marker_file} marker file to be in source datadir: $source_datadir"
            [ -f "${source_datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $source_datadir"

            # ensure the target cluster is located where the source used to be
            [ -d "${datadir}/" ] || fail "expected target data directory to be located at $datadir"
            [ -f "${datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $datadir"
            [ ! -f "${datadir}/${marker_file}" ] || fail "unexpected ${marker_file} marker file in target datadir: $datadir"
        done
}

validate_tablespace_dirs() {
        TABLESPACERDIRS=("$@")
        for tablespaceDir in "${TABLESPACERDIRS[@]}"; do
            [ -d "${tablespaceDir}" ] || fail "expected tablespace directory to be located at $tablespaceDir"
        done
}

validate_pg_hba_conf() {
    local HBA_HOSTNAMES=$1
    local expected_hosts=()
    local actual_hosts=()
    local matched=()
    local unmatched=()

    # shellcheck disable=SC2207
    local expected_hosts=( $(all_hosts) )
    # shellcheck disable=SC2207
    IFS=$'\n' expected_hosts=( $(sort <<<"${expected_hosts[*]}") )
    # shellcheck disable=SC2207
    IFS=$'\n' expected_hosts=( $(uniq <<<"${expected_hosts[*]}") )
    unset IFS

    for datadir in "${datadirs[@]}"; do
        # shellcheck disable=SC2207
        actual_hosts=( $(grep  -v '^#' "${datadir}/pg_hba.conf" | grep -v '^$' | grep -v '^local' | awk '{ print $4 }' | sort | uniq) )
        for expected_host in "${expected_hosts[@]}"; do
            for actual_host in "${actual_hosts[@]}"; do
                if [ "$actual_host" == "$expected_host" ]; then
                    matched+=( "$actual_host" )
                    break 2
                fi
            done
            unmatched+=( "$expected_host" )
        done

        if [[ -n "$HBA_HOSTNAMES" ]] && (( ${#unmatched[@]} )); then
            log "expected ${datadir}/pg_hba.conf to contain all hosts '${expected_hosts[*]}'. Found '${actual_hosts[*]}'"
        fi

        if [[ -z "$HBA_HOSTNAMES" ]] && (( ${#matched[@]} )); then
            log "expected ${datadir}/pg_hba.conf to 'not' contain any of the hosts '${expected_hosts[*]}'. Found '${actual_hosts[*]}'"
        fi
    done
}
