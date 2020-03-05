#!/usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    PSQL="$GPHOME/bin/psql --no-align --tuples-only postgres"

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -n "${BATS_TEST_SKIPPED}" ]; then
        return
    fi

    teardown_new_cluster
    gpupgrade kill-services

    # reload old path and start
    source "${GPHOME}/greenplum_path.sh"
    gpstart -a
}

@test "gpupgrade finalize should swap the target data directories and ports with the source cluster" {
    # place marker file in source master data directory
    local marker_file=source-cluster.test-marker
    local datadirs=($(get_datadirs))
    for datadir in "${datadirs[@]}"; do
        touch "$datadir/${marker_file}"
    done

    # grab the original configuration before starting so we can verify the
    # target cluster ends up with the source cluster's original layout
    local old_config=$(get_segment_configuration)

    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --verbose 3>&-

    gpupgrade execute --verbose
    gpupgrade finalize --verbose

    for datadir in "${datadirs[@]}"; do
        # ensure the source cluster has been archived
        local source_datadir=$(dirname ${datadir})"_old/$(basename ${datadir})"
        if [ "$(basename ${datadir})" == "standby" ]; then
            # Standby follows different naming rules
            source_datadir="${datadir}_old"
        fi

        [ -d "${source_datadir}/" ] || fail "expected source data directory to be located at $source_datadir"
        [ -f "${source_datadir}/${marker_file}" ] || fail "expected ${marker_file} marker file to be in source datadir: $source_datadir"
        [ -f "${source_datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $source_datadir"

        # ensure the target cluster is located where the source used to be
        [ -d "${datadir}/" ] || fail "expected target data directory to be located at $datadir"
        [ -f "${datadir}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $datadir"
        [ ! -f "${datadir}/${marker_file}" ] || fail "unexpected ${marker_file} marker file in target datadir: $datadir"
    done

    # ensure gpperfmon configuration file has been modified to reflect new data dir location
    local gpperfmon_config_file="${MASTER_DATA_DIRECTORY}/gpperfmon/conf/gpperfmon.conf"
    grep "${MASTER_DATA_DIRECTORY}" "${gpperfmon_config_file}" || \
        fail "got gpperfmon.conf file $(cat $gpperfmon_config_file), wanted it to include ${MASTER_DATA_DIRECTORY}"

    # Check to make sure the new cluster matches the old one.
    local new_config=$(get_segment_configuration)
    [ "$old_config" = "$new_config" ] || fail "actual config: $new_config, wanted $old_config"

    local new_datadir=$(gpupgrade config show --new-datadir)
    # TODO: Query gp_stat_replication to check if the standby is in sync. Since
    # this is a more accurate representation if the standby is running and
    # in sync, since gpstate might simply check if the process is running.
    local actual_standby_status=$(gpstate -d "${new_datadir}")
    local standby_status_line=$(get_standby_status "$actual_standby_status")
    [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"
}

setup_state_dir() {
    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

teardown_new_cluster() {
    delete_finalized_cluster $MASTER_DATA_DIRECTORY
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

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}
