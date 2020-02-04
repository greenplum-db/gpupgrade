#!/usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
    PSQL="$GPHOME"/bin/psql
    TEARDOWN_FUNCTIONS=()
}

teardown() {
    skip_if_no_gpdb
    $PSQL postgres -c "drop table if exists test_linking;"

    gpupgrade kill-services
    rm -r "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $NEW_CLUSTER
    fi

    for FUNCTION in "${TEARDOWN_FUNCTIONS[@]}"; do
        $FUNCTION
    done

    gpstart -a
}

ensure_hardlinks_for_relfilenode_on_master_and_segments() {
    local tablename=$1
    local expected_number_of_hardlinks=$2

    read -r -a relfilenodes <<< $($PSQL postgres --tuples-only --no-align -c "
        CREATE FUNCTION pg_temp.seg_relation_filepath(tbl text)
            RETURNS TABLE (dbid int, path text)
            EXECUTE ON ALL SEGMENTS
            LANGUAGE SQL
        AS \$\$
            SELECT current_setting('gp_dbid')::int, pg_relation_filepath(tbl);
        \$\$;
        CREATE FUNCTION pg_temp.gp_relation_filepath(tbl text)
            RETURNS TABLE (dbid int, path text)
            LANGUAGE SQL
        AS \$\$
            SELECT current_setting('gp_dbid')::int, pg_relation_filepath(tbl)
                UNION ALL SELECT * FROM pg_temp.seg_relation_filepath(tbl);
        \$\$;
        SELECT c.datadir || '/' || f.path
          FROM pg_temp.gp_relation_filepath('$tablename') f
          JOIN gp_segment_configuration c
            ON c.dbid = f.dbid;
    ")

    for relfilenode in "${relfilenodes[@]}"; do
        local number_of_hardlinks=$($STAT --format "%h" "${relfilenode}")
        [ $number_of_hardlinks -eq $expected_number_of_hardlinks ] \
            || fail "expected $expected_number_of_hardlinks hardlinks to $relfilenode; found $number_of_hardlinks"
    done
}

set_master_and_primary_datadirs() {
    run $PSQL -At -p $PGPORT postgres -c "SELECT datadir FROM gp_segment_configuration WHERE role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    master_and_primary_datadirs=("${lines[@]}")
}

reset_master_and_primary_pg_control_files() {
    for datadir in "${master_and_primary_datadirs[@]}"; do
        mv "${datadir}/global/pg_control.old" "${datadir}/global/pg_control"
    done
}

@test "gpupgrade execute should remember that link mode was specified in initialize" {
    require_gnu_stat
    set_master_and_primary_datadirs

    delete_target_datadirs "${MASTER_DATA_DIRECTORY}"

    $PSQL postgres -c "drop table if exists test_linking; create table test_linking (a int);"

    ensure_hardlinks_for_relfilenode_on_master_and_segments 'test_linking' 1

    gpupgrade initialize \
        --old-bindir="$GPHOME/bin" \
        --new-bindir="$GPHOME/bin" \
        --old-port="${PGPORT}" \
        --link \
        --disk-free-ratio 0 \
        --verbose

    local datadir=$(dirname $(dirname "${MASTER_DATA_DIRECTORY}"))
    NEW_CLUSTER="${datadir}/qddir_upgrade/demoDataDir-1"

    gpupgrade execute --verbose
    TEARDOWN_FUNCTIONS+=( reset_master_and_primary_pg_control_files )

    PGPORT=50432 ensure_hardlinks_for_relfilenode_on_master_and_segments 'test_linking' 2
}

@test "gpupgrade execute step to upgrade master should always rsync the master data dir from backup" {
    require_gnu_stat
    set_master_and_primary_datadirs

    delete_target_datadirs "${MASTER_DATA_DIRECTORY}"

    gpupgrade initialize \
        --old-bindir="$GPHOME/bin" \
        --new-bindir="$GPHOME/bin" \
        --old-port="${PGPORT}" \
        --link \
        --disk-free-ratio 0 \
        --verbose

    local datadir=$(dirname $(dirname "${MASTER_DATA_DIRECTORY}"))
    NEW_CLUSTER="${datadir}/qddir_upgrade/demoDataDir-1"

    # Initialize creates a backup of the target master data dir, during execute
    # upgrade master steps refreshes the content of the target master data dir
    # with the existing backup. Remove the target master data directory to
    # ensure that initialize created a backup and upgrade master refreshed the
    # target master data directory with the backup.
    rm -rf "${datadir}"/qddir_upgrade/demoDataDir-1/*
    
    # create an extra file to ensure that its deleted during rsync as we pass
    # --delete flag
    mkdir "${datadir}"/qddir_upgrade/demoDataDir-1/base_extra    
    touch "${datadir}"/qddir_upgrade/demoDataDir-1/base_extra/1101
    gpupgrade execute --verbose
    
    # check that the extraneous files are deleted
    [ ! -d "${datadir}"/qddir_upgrade/demoDataDir-1/base_extra ]

    TEARDOWN_FUNCTIONS+=( reset_master_and_primary_pg_control_files )

}
