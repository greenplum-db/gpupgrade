# This file provides a single high-level function check_mirror_validity()
# that takes a cluster with mirrors "through its paces" to thoroughly test
# the cluster's mirrors.  See the documentation of check_mirror_validity()
# for details.

check_mirrors() {
    _check_segments_are_synchronized
    _check_mirror_replication_connections
}

_check_segments_are_synchronized() {
    for i in {1..10}; do
        local unsynced=$(ssh -n "${MASTER_HOST}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            psql -p $MASTER_PORT -At -d postgres << EOF
                SELECT gp_request_fts_probe_scan();
                SELECT count(*) FROM gp_segment_configuration WHERE content <> -1 AND mode = 'n';
EOF
        " | tail -1)
        if [ "$unsynced" = "0" ]; then
            return 0
        fi
        sleep 5
    done

    echo "failed to synchronize within time limit"
    return 1
}

_check_mirror_replication_connections() {
    local rows=$(ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -p $MASTER_PORT -d postgres -AtF$'\t' -c \"
            SELECT primaries.address, primaries.port, mirrors.hostname
            FROM gp_segment_configuration AS primaries
            JOIN gp_segment_configuration AS mirrors
            ON primaries.content = mirrors.content
            WHERE primaries.role = 'p' AND mirrors.role = 'm' AND primaries.content != -1;
        \"
    ")

    echo "${rows}" | while read -r primary_address primary_port mirror_host; do
        ssh -n "${mirror_host}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            PGOPTIONS=\"-c gp_session_role=utility\" psql -v ON_ERROR_STOP=1 -h $primary_address -p $primary_port \"dbname=postgres replication=database\" -c \"
                IDENTIFY_SYSTEM;
            \"
        " || return $?
    done
}

kill_primaries() {
    local primaries
    primaries=$(ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -AtF$'\t' -p $MASTER_PORT -d postgres -c \"
            SELECT hostname, port, datadir FROM gp_segment_configuration
            WHERE content <> -1 AND role = 'p'
        \"
    ") || return $?

    echo "${primaries}" | while read -r host port dir; do
        ssh -n "${host}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            pg_ctl stop -p $port -m immediate -D $dir -w
        "
    done
}

wait_can_start_transactions() {
    local host=$1
    local port=$2
    for i in {1..10}; do
        ssh -n "${host}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            psql -p $port -At -d postgres << EOF
                SELECT gp_request_fts_probe_scan();
                BEGIN; CREATE TEMP TABLE temp_test(a int) DISTRIBUTED RANDOMLY; COMMIT;
EOF
        "
        if [ $? -eq 0 ]; then
            return 0
        fi
        sleep 5
    done

    echo "failed to start transactions within time limit"
    return 1
}

# After creating the new table, this function outputs its distribution to stdout.
create_table_with_name() {
    local table_name=$1
    local size=$2
    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        # -q suppresses all output from this command
        psql -q -p $MASTER_PORT -d postgres <<EOF
            CREATE TABLE ${table_name} (a int) DISTRIBUTED BY (a);
            INSERT INTO ${table_name} SELECT * FROM generate_series(0,${size});
EOF
    "
    _get_data_distribution $table_name
}

_get_data_distribution() {
    local table_name=$1
    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -t -A -p $MASTER_PORT -d postgres -c \"
            SELECT gp_segment_id,count(*) FROM ${table_name}
            GROUP BY gp_segment_id ORDER BY gp_segment_id;
        \"
    "
}

check_data_matches() {
    local table_name=$1
    local expected=$2

    local actual=$(_get_data_distribution $table_name)
    if [ "${actual}" != "${expected}" ]; then
        echo "Checking table ${table_name} - got: ${actual} want: ${expected}"
        return 1
    fi
}

# Check the validity of the upgraded mirrors - failover to them and then recover, similar to cross-subnet testing
# |  step  |   mdw       | smdw         | sdw-primaries | sdw-mirrors |
# |    1   |   master    |   standby    |    primary    |  mirror     |
# |    2   |   master    |   standby    |      -        |  mirror     |
# |    3   |   master    |   standby    |      -        |  primary    |
# |    4   |   master    |   standby    |   mirror      |  primary    |
# |    5   |   master    |   standby    |   primary     |  mirror     |
check_mirror_validity() {
    GPHOME_NEW=$1
    MASTER_HOST=$2
    MASTER_PORT=$3

    local master_data_dir=$(ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -p $MASTER_PORT -t -A -d postgres -c \"
            SELECT datadir FROM gp_segment_configuration
            WHERE content = -1 AND role = 'p'
        \"
    ")

    # step 1
    wait_can_start_transactions $MASTER_HOST $MASTER_PORT
    check_mirrors

    local on_upgraded_master_expected=$(create_table_with_name on_upgraded_master 50)

    # step 2
    kill_primaries

    # step 3
    wait_can_start_transactions $MASTER_HOST $MASTER_PORT

    check_data_matches on_upgraded_master "${on_upgraded_master_expected}"
    local on_promoted_mirrors_expected=$(create_table_with_name on_promoted_mirrors 60)

    # step 4
    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=${master_data_dir}
        export PGPORT=$MASTER_PORT
        gprecoverseg -a       # TODO..why is PGPORT not actually needed here?
    "
    check_mirrors

    check_data_matches on_upgraded_master "${on_upgraded_master_expected}"
    check_data_matches on_promoted_mirrors "${on_promoted_mirrors_expected}"
    local on_recovered_cluster_expected=$(create_table_with_name on_recovered_cluster 70)

    # step 5
    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=${master_data_dir}
        export PGPORT=$MASTER_PORT
        gprecoverseg -ra
    "
    check_mirrors

    check_data_matches on_upgraded_master "${on_upgraded_master_expected}"
    check_data_matches on_promoted_mirrors "${on_promoted_mirrors_expected}"
    check_data_matches on_recovered_cluster "${on_recovered_cluster_expected}"
}

