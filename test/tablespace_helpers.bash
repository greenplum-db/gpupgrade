# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

#
# Helpers for testing and verifying tablespaces.
#

# Prints a line for each segment containing the hostname, dbid, and datadir,
# separated by tabs.
_query_5X_host_dbid_datadirs() {
    "$GPHOME_SOURCE"/bin/psql -AtF$'\t' postgres -c "
        SELECT s.hostname,
               s.dbid,
               e.fselocation
          FROM gp_segment_configuration s
          JOIN pg_filespace_entry e ON (e.fsedbid = s.dbid)
          JOIN pg_filespace f       ON (f.oid = e.fsefsoid)
         WHERE f.fsname = 'pg_system'
         ORDER BY dbid;
    "
}

# This is 5X-only due to a bug in pg_upgrade for 6-6
create_tablespace_with_tables() {
    local tablespace_table_prefix=${1:-batsTable}
    local tablespace_dir entries

    # the tablespace directory will get deleted when the STATE_DIR is deleted in teardown()
    TABLESPACE_ROOT="${STATE_DIR}"/testfs
    TABLESPACE_CONFIG="${TABLESPACE_ROOT}"/fs.txt

    # create the filespace config file and the directories required to implement it
    entries=$(_query_5X_host_dbid_datadirs)

    mkdir "$TABLESPACE_ROOT"
    echo "filespace:batsFS" > "$TABLESPACE_CONFIG"

    local host dbid datadir
    while read -r host dbid datadir; do
        # if symlink length is > 100, it will get trimmed and result in an invalid symlink when pg_basebackup will
        # copy the master to standby, so keep them short
        tablespace_dir="${TABLESPACE_ROOT}/${dbid}"/$(basename "$datadir")

        ssh -n "$host" mkdir -p "$(dirname "$tablespace_dir")"
        echo "${host}:${dbid}:${tablespace_dir}" >> "$TABLESPACE_CONFIG"
    done <<< "$entries"

    # Print out the config to help debug problems.
    echo "tablespace configuration:"
    cat "$TABLESPACE_CONFIG"

    (source "${GPHOME_SOURCE}"/greenplum_path.sh && gpfilespace --config "${TABLESPACE_CONFIG}")

    # create a tablespace in said filespace and a table in that tablespace
    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				CREATE TABLESPACE batsTbsp FILESPACE batsFS;
				CREATE TABLE "${tablespace_table_prefix}_0" (a int) TABLESPACE batsTbsp;
				INSERT INTO "${tablespace_table_prefix}_0" SELECT i from generate_series(1,100)i;

				CREATE TABLE "${tablespace_table_prefix}_1" (a int) WITH(appendonly=true, orientation=row) TABLESPACE batsTbsp;
				INSERT INTO "${tablespace_table_prefix}_1" SELECT i from generate_series(1,100)i;

				CREATE TABLE "${tablespace_table_prefix}_2" (a int) WITH(appendonly=true, orientation=column) TABLESPACE batsTbsp;
				INSERT INTO "${tablespace_table_prefix}_2" SELECT i from generate_series(1,100)i;

				CREATE TABLE  "${tablespace_table_prefix}_3" (a int, b int) WITH(appendonly=true, orientation=column) TABLESPACE batsTbsp
				    PARTITION BY RANGE(b) (START(1) END(4) EVERY(1));
				INSERT INTO "${tablespace_table_prefix}_3" SELECT i, (i%3)+1 FROM generate_series(1,100)i;
EOF
}

# This is 5X-only
delete_tablespace_data() {
   local tablespace_table_prefix=${1:-batsTable}

    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				DROP TABLE IF EXISTS "${tablespace_table_prefix}_0";
				DROP TABLE IF EXISTS "${tablespace_table_prefix}_1";
				DROP TABLE IF EXISTS "${tablespace_table_prefix}_2";
				DROP TABLE IF EXISTS "${tablespace_table_prefix}_3";
				DROP TABLESPACE IF EXISTS batsTbsp;
				DROP FILESPACE IF EXISTS batsFS;
EOF
}

truncate_tablespace_data() {
       local tablespace_table_prefix=${1:-batsTable}
       local port=$2

    "${GPHOME_SOURCE}"/bin/psql -p "$port" -d postgres -v ON_ERROR_STOP=1 <<- EOF
				TRUNCATE "${tablespace_table_prefix}_0";
				TRUNCATE "${tablespace_table_prefix}_1";
				TRUNCATE "${tablespace_table_prefix}_2";
				TRUNCATE "${tablespace_table_prefix}_3";
EOF
}

check_tablespace_data() {
    local tablespace_table_prefix=${1:-batsTable}

    local rows
    for table in "${tablespace_table_prefix}_0" "${tablespace_table_prefix}_1" "${tablespace_table_prefix}_2" "${tablespace_table_prefix}_3"; do
        rows=$("$GPHOME_TARGET"/bin/psql -d postgres -Atc "SELECT COUNT(*) FROM \"$table\";")
        if (( rows != 100 )); then
            fail "failed verifying tablespaces. $table got $rows want 100"
        fi
    done
}
