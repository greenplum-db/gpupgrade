#!/usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    [ ! -z $GPHOME ]
    GPHOME_NEW=${GPHOME_NEW:-$GPHOME}
    GPHOME_OLD=$GPHOME

    PSQL="$GPHOME_NEW/bin/psql --no-align --tuples-only postgres"

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    skip_if_no_gpdb

    teardown_new_cluster
    gpupgrade kill-services

    # reload old path and start
    source "${GPHOME_OLD}/greenplum_path.sh"
    gpstart -a
}

@test "gpupgrade finalize should swap the target data directories and ports with the source cluster" {
    # place marker file in source master data directory
    local marker_file=source-cluster.test-marker
    touch "$MASTER_DATA_DIRECTORY/${marker_file}"

    # grab the original ports before starting so we can verify the target cluster
    # inherits the source cluster's ports
    local old_ports=$(get_ports)

    gpupgrade initialize \
        --old-bindir="$GPHOME/bin" \
        --new-bindir="$GPHOME_NEW/bin" \
        --old-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --verbose

    gpupgrade execute --verbose

    gpupgrade finalize

    # ensure the source cluster has been archived
    local source_cluster_master_data_directory=$(dirname ${MASTER_DATA_DIRECTORY})"_old/demoDataDir-1"
    [ -d "${source_cluster_master_data_directory}/" ] || fail "expected source data directory to be located at $source_cluster_master_data_directory"
    [ -f "${source_cluster_master_data_directory}/${marker_file}" ] || fail "expected ${marker_file} marker file to be in source datadir: $source_cluster_master_data_directory"
    [ -f "${source_cluster_master_data_directory}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $source_cluster_master_data_directory"

    # ensure the target cluster is located where the source used to be
    local target_cluster_master_data_directory="${MASTER_DATA_DIRECTORY}"
    [ -d "${target_cluster_master_data_directory}/" ] || fail "expected target data directory to be located at $target_cluster_master_data_directory"
    [ -f "${target_cluster_master_data_directory}/postgresql.conf" ] || fail "expected postgresql.conf file to be in $target_cluster_master_data_directory"
    [ ! -f "${target_cluster_master_data_directory}/${marker_file}" ] || fail "unexpected ${marker_file} marker file in target datadir: $target_cluster_master_data_directory"

    # ensure gpperfmon configuration file has been modified to reflect new data dir location
    local gpperfmon_config_file="${target_cluster_master_data_directory}/gpperfmon/conf/gpperfmon.conf"
    grep "${target_cluster_master_data_directory}" "${gpperfmon_config_file}" || \
        fail "got gpperfmon.conf file $(cat $gpperfmon_config_file), wanted it to include ${target_cluster_master_data_directory}"

    # ensure that the new cluster is queryable, and has updated configuration
    segment_configuration=$($PSQL -c "select *, version() from gp_segment_configuration")
    [[ $segment_configuration == *"$target_cluster_master_data_directory"* ]] || fail "expected $segment_configuration to include $target_cluster_master_data_directory"

    # Check to make sure the new cluster's ports match the old one.
    local new_ports=$(get_ports)
    [ "$old_ports" = "$new_ports" ] || fail "actual ports: $new_ports, wanted $old_ports"

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

# Writes the primary ports from the cluster pointed to by $PGPORT to stdout, one
# per line, sorted by content ID.
get_ports() {
    $PSQL -c "select content, role, port from gp_segment_configuration where role = 'p' order by content, role"
}

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}
