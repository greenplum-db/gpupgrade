#!/usr/bin/env bats
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    $PSQL postgres -c "drop table if exists test_linking;"

    gpupgrade kill-services
    archive_state_dir "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi

    start_source_cluster
}

ensure_hardlinks_for_relfilenode_on_master_and_segments() {
    local gphome=$1
    local port=$2
    local tablename=$3
    local expected_number_of_hardlinks=$4

    local sql="
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
        ON c.dbid = f.dbid;"

    if is_GPDB5 "$gphome"; then
        sql="
        SELECT e.fselocation||'/'||'base'||'/'||d.oid||'/'||c.relfilenode
          FROM gp_segment_configuration s
          JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
          JOIN pg_filespace f ON e.fsefsoid = f.oid
          JOIN pg_database d ON d.datname=current_database()
          JOIN gp_dist_random('pg_class') c ON c.gp_segment_id = s.content
        WHERE f.fsname = 'pg_system' AND role = 'p'
              AND c.relname = '$tablename'
        UNION ALL
        SELECT e.fselocation||'/'||'base'||'/'||d.oid||'/'||c.relfilenode
          FROM gp_segment_configuration s
          JOIN pg_filespace_entry e ON s.dbid = e.fsedbid
          JOIN pg_filespace f ON e.fsefsoid = f.oid
          JOIN pg_database d ON d.datname=current_database()
          JOIN pg_class c ON c.gp_segment_id = s.content
        WHERE f.fsname = 'pg_system' AND role = 'p'
        AND c.relname = '$tablename';"
    fi

    read -r -a relfilenodes <<< $("$gphome"/bin/psql postgres -p "$port" --tuples-only --no-align -c "$sql")

    for relfilenode in "${relfilenodes[@]}"; do
        local number_of_hardlinks=$($STAT --format "%h" "${relfilenode}")
        [ $number_of_hardlinks -eq $expected_number_of_hardlinks ] \
            || fail "expected $expected_number_of_hardlinks hardlinks to $relfilenode; found $number_of_hardlinks"
    done
}

@test "gpupgrade execute should remember that link mode was specified in initialize" {
    require_gnu_stat
    setup_restore_cluster "--mode=link"

    delete_target_datadirs "${MASTER_DATA_DIRECTORY}"

    $PSQL postgres -c "drop table if exists test_linking; create table test_linking (a int);"

    ensure_hardlinks_for_relfilenode_on_master_and_segments $GPHOME_SOURCE $PGPORT 'test_linking' 1

    gpupgrade initialize \
        --automatic \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --mode="link" \
        --disk-free-ratio 0 \
        --verbose

    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    gpupgrade execute --non-interactive --verbose

    ensure_hardlinks_for_relfilenode_on_master_and_segments $GPHOME_TARGET 6020 'test_linking' 2

    restore_cluster
}

@test "gpupgrade execute step to upgrade master should always rsync the master data dir from backup" {
    require_gnu_stat
    setup_restore_cluster "--mode=link"

    delete_target_datadirs "${MASTER_DATA_DIRECTORY}"

    gpupgrade initialize \
        --automatic \
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

    # Initialize creates a backup of the target master data dir, during execute
    # upgrade master steps refreshes the content of the target master data dir
    # with the existing backup. Remove the target master data directory to
    # ensure that initialize created a backup and upgrade master refreshed the
    # target master data directory with the backup.
    abort_unless_target_master "${datadir}"
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
        --automatic \
        --source-gphome="$GPHOME_SOURCE" \
        --target-gphome="$GPHOME_TARGET" \
        --source-master-port="${PGPORT}"\
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    NEW_CLUSTER="$(gpupgrade config show --target-datadir)"

    gpupgrade execute --non-interactive --verbose 3>&-

    # On GPDB5, restore the primary and master directories before starting the cluster
    restore_cluster

    # Put the source and target clusters back the way they were.
    # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    (unset LD_LIBRARY_PATH; source "$GPHOME_TARGET"/greenplum_path.sh && gpstop -a -d "$NEW_CLUSTER")
    start_source_cluster

    # Mark every substep in the status file as failed. Then re-execute.
    sed -i.bak -e 's/"COMPLETE"/"FAILED"/g' "$GPUPGRADE_HOME/substeps.json"

    gpupgrade execute --non-interactive --verbose 3>&-

    restore_cluster
}
