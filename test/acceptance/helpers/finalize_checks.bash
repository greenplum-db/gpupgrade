# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# This file provides a single high-level function validate_mirrors_and_standby()
# that takes a cluster with mirrors and a standby "through its paces" to
# thoroughly test those mirrors and standby.

check_synchronized_cluster() {
    local master_host=$1
    local master_port=$2

    for i in {1..10}; do
        local synced
        synced=$(ssh -n "$master_host" "
            source ${GPHOME_NEW}/greenplum_path.sh
            psql -X -p $master_port -At -d postgres << EOF
                SELECT gp_request_fts_probe_scan();
                SELECT EVERY(state='streaming' AND state IS NOT NULL)
                FROM gp_stat_replication;
EOF
        " | tail -1)
        if [ "$synced" = "t" ]; then
            return 0
        fi
        sleep 5
    done

    echo "failed to synchronize within time limit"
    return 1
}

check_replication_connections() {
    local host=$1
    local port=$2

    local rows
    rows=$(ssh -n "${host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -X -p $port -d postgres -AtF$'\t' -c \"
            SELECT primaries.address, primaries.port, mirrors.hostname
            FROM gp_segment_configuration AS primaries
            JOIN gp_segment_configuration AS mirrors
            ON primaries.content = mirrors.content
            WHERE primaries.role = 'p' AND mirrors.role = 'm';
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

wait_can_start_transactions() {
    local host=$1
    local port=$2

    for i in {1..10}; do
        ssh -n "${host}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            psql -X -p $port -At -d postgres << EOF
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

stop_segments_with_contents() {
    local filter="content $1"
    local host=$2
    local port=$3

    local contents
    contents=$(ssh -n "$host" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -X -AtF$'\t' -p $port -d postgres -c \"
            SELECT hostname, port, datadir FROM gp_segment_configuration
            WHERE $filter AND role = 'p'
        \"
    ") || return $?

    echo "${contents}" | while read -r host port dir; do
        ssh -n "${host}" "
            source ${GPHOME_NEW}/greenplum_path.sh
            pg_ctl stop -p $port -m immediate -D $dir -w
        "
    done
}

# After creating the new table, this function outputs its distribution to stdout.
create_table_with_name() {
    local table_name=$1
    local size=$2
    local host=$3
    local port=$4

    ssh -n "${host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        # -q suppresses all output from this command
        psql -X -v ON_ERROR_STOP=1 -q -p $port -d postgres <<EOF
            CREATE TABLE ${table_name} (a int) DISTRIBUTED BY (a);
            INSERT INTO ${table_name} SELECT * FROM generate_series(1,${size});
EOF
    " || return $?
    _get_data_distribution $host $port $table_name
}

_get_data_distribution() {
    local host=$1
    local port=$2
    local table_name=$3

    ssh -n "${host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -v ON_ERROR_STOP=1 -t -A -p $port -d postgres -c \"
            SELECT gp_segment_id,count(*) FROM ${table_name}
            GROUP BY gp_segment_id ORDER BY gp_segment_id;
        \"
    "
}

check_data_matches() {
    local table_name=$1
    local expected=$2
    local host=$3
    local port=$4

    local actual
    actual=$(_get_data_distribution $host $port $table_name)
    if [ "${actual}" != "${expected}" ]; then
        echo "Checking table ${table_name} - got: ${actual} want: ${expected}"
        return 1
    fi
}

contents_without_mirror() {
    local gphome=$1
    local host=$2
    local port=$3

    ssh -n "$host" "
        source ${gphome}/greenplum_path.sh
        psql -X -p $port -At -d postgres -c \"
            SELECT content
            FROM gp_segment_configuration
            GROUP BY content
            HAVING COUNT(*)!=2;
        \"
    "
}

# |     step                        | mdw     | smdw    | sdw-p   | sdw-m   |
# |---------------------------------|---------|---------|---------|---------|
# | 1:  initial                     | master  | standby | primary | mirror  |
# | 2a: failover stop               | -       | standby |   -     | mirror  |
# | 2b: failover promote            | -       | master  |   -     | primary |
# | 3:  restore mirrors and standby | standby | master  | mirror  | primary |
# | 4a: rebalance mirrors           | standby | master  | primary | mirror  |
# | 4b: rebalance standby           | standby |     -   | primary | mirror  |
# | 4c: rebalance standby           | master  |     -   | primary | mirror  |
# | 4d: rebalance standby           | master  | standby | primary | mirror  |
#
# For rebalancing the standby, we followed these instructions:
# https://gpdb.docs.pivotal.io/6-4/admin_guide/highavail/topics/g-restoring-master-mirroring-after-a-recovery.html#topic17
#
# NOTE: when in a given step of this test, keep in mind that the master
#  switches back and forth between the mdw host("MASTER") and the smdw host("standby").

validate_mirrors_and_standby() {
    GPHOME_NEW=$1
    local MASTER_HOST=$2
    local MASTER_PORT=$3

    local noMirrors
    noMirrors=$(contents_without_mirror "$GPHOME_NEW" "$MASTER_HOST" "$MASTER_PORT")
    if [ -n "$noMirrors" ]; then
        echo "This test only works on full clusters but these content ids do not have mirrors: ${noMirrors}"
        exit 1
    fi

    local master_data_dir
    master_data_dir=$(ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -X -p $MASTER_PORT -At -d postgres -c \"
            SELECT datadir FROM gp_segment_configuration
            WHERE content = -1 AND role = 'p'
        \"
    ")

    local standby_info
    standby_info=$(ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        psql -X -p $MASTER_PORT -AtF$'\t' -d postgres -c \"
            SELECT hostname, port, datadir FROM gp_segment_configuration
            WHERE content = -1 AND role = 'm'
        \"
    ")
    read -r standby_host standby_port standby_data_dir <<<"${standby_info}"

    # step 1: initial
    check_replication_connections "${MASTER_HOST}" "${MASTER_PORT}"
    check_synchronized_cluster "${MASTER_HOST}" "${MASTER_PORT}"
    wait_can_start_transactions "${MASTER_HOST}" "${MASTER_PORT}"

    local data_on_upgraded_cluster
    data_on_upgraded_cluster=$(create_table_with_name on_upgraded_cluster 50 "${MASTER_HOST}" "${MASTER_PORT}")

    # step 2a: failover stop...
    # FIXME: We should be able to stop both the master and primaries at once
    # with ">=-1". However, there appears to be a bug where the standby does not
    # have the correct or latest information after being promoted. The standby
    # has the table, and the segments have the data. But checking the data shows
    # nothing.
    stop_segments_with_contents ">-1" "${MASTER_HOST}" "${MASTER_PORT}"
    wait_can_start_transactions "${MASTER_HOST}" "${MASTER_PORT}"
    stop_segments_with_contents "=-1" "${MASTER_HOST}" "${MASTER_PORT}"

    # step 2b: failover promote...
    ssh -n "${standby_host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export PGPORT=$standby_port
        gpactivatestandby -a -d $standby_data_dir
    "
    wait_can_start_transactions "${standby_host}" "${standby_port}"

    check_data_matches on_upgraded_cluster "${data_on_upgraded_cluster}" "${standby_host}" "${standby_port}"
    local data_on_promoted_cluster
    data_on_promoted_cluster=$(create_table_with_name on_promoted_cluster 60 "${standby_host}" "${standby_port}")

    # step 3:  restore mirrors and standby
    ssh -n "${standby_host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=${standby_data_dir}
        export PGPORT=$standby_port
        gprecoverseg -a       # TODO..why is PGPORT not actually needed here?
    "
    wait_can_start_transactions $standby_host "${standby_port}"  #TODO: is this necessary?

    # sanity check both the demo cluster and CI cluster cases
    if [[ $master_data_dir != *datadirs/qddir/demoDataDir* && $master_data_dir != */data/gpdata/master/gpseg-1* ]]; then
        echo "cowardly refusing to delete $master_data_dir which does not look like a demo or CI master data dir"
        exit 1
    fi
    ssh -n "${MASTER_HOST}" "rm -r ${master_data_dir}"

    ssh -n "${standby_host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export PGPORT=$standby_port; gpinitstandby -a -s $MASTER_HOST -P $MASTER_PORT -S $master_data_dir
    "
    check_replication_connections "${standby_host}" "${standby_port}"
    check_synchronized_cluster "${standby_host}" "${standby_port}"

    check_data_matches on_upgraded_cluster "${data_on_upgraded_cluster}" "${standby_host}" "${standby_port}"
    check_data_matches on_promoted_cluster "${data_on_promoted_cluster}" "${standby_host}" "${standby_port}"
    local data_on_unbalanced_cluster
    data_on_unbalanced_cluster=$(create_table_with_name  on_unbalanced_cluster 70 "${standby_host}" "${standby_port}")

    # 4a: rebalance mirrors
    ssh -n "${standby_host}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export MASTER_DATA_DIRECTORY=${standby_data_dir}
        export PGPORT=$standby_port
        gprecoverseg -ra
    "
    check_replication_connections "${standby_host}" "${standby_port}"
    check_synchronized_cluster "${standby_host}" "${standby_port}"


    # 4b: rebalance standby
    stop_segments_with_contents "=-1" "${standby_host}" "${standby_port}"

    # 4c: rebalance standby
    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export PGPORT=$MASTER_PORT
        gpactivatestandby -a -d $master_data_dir
    "

    # 4d: rebalance standby

    # sanity check both the demo cluster and CI cluster cases
    if [[ $standby_data_dir != *datadirs/standby* && $standby_data_dir != */data/gpdata/master/gpseg-1* ]]; then
        echo "cowardly refusing to delete $standby_data_dir which does not look like a demo or CI standby data dir"
        exit 1
    fi
    ssh -n "${standby_host}" "rm -r $standby_data_dir"

    ssh -n "${MASTER_HOST}" "
        source ${GPHOME_NEW}/greenplum_path.sh
        export PGPORT=$MASTER_PORT; gpinitstandby -a -s $standby_host -P $standby_port -S $standby_data_dir
    "
    check_replication_connections "${MASTER_HOST}" "${MASTER_PORT}"
    check_synchronized_cluster "${MASTER_HOST}" "${MASTER_PORT}"

    check_data_matches on_upgraded_cluster "${data_on_upgraded_cluster}" "${MASTER_HOST}" "${MASTER_PORT}"
    check_data_matches on_promoted_cluster "${data_on_promoted_cluster}" "${MASTER_HOST}" "${MASTER_PORT}"
    check_data_matches on_unbalanced_cluster "${data_on_unbalanced_cluster}" "${MASTER_HOST}" "${MASTER_PORT}"
}


