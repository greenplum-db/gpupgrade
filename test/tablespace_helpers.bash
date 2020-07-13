# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

#
# Helpers for testing and verifying tablespaces.
#

# This is 5X-only due to a bug in pg_upgrade for 6-6
create_tablespace_with_table() {
    # the tablespace directory will get deleted when the STATE_DIR is deleted in teardown()
    TABLESPACE_ROOT="${STATE_DIR}"/testfs
    TABLESPACE_CONFIG="${TABLESPACE_ROOT}"/fs.txt

    # create the directories required to implement our filespace
    mkdir -p "${TABLESPACE_ROOT}"/{m,{p,m}{1,2,3}}

    # create the filespace config file
    cat <<- EOF > "$TABLESPACE_CONFIG"
				filespace:batsFS
				$(hostname):1:${TABLESPACE_ROOT}/m/demoDataDir-1
				$(hostname):2:${TABLESPACE_ROOT}/p1/demoDataDir0
				$(hostname):3:${TABLESPACE_ROOT}/p2/demoDataDir1
				$(hostname):4:${TABLESPACE_ROOT}/p3/demoDataDir2
				$(hostname):5:${TABLESPACE_ROOT}/m1/demoDataDir0
				$(hostname):6:${TABLESPACE_ROOT}/m2/demoDataDir1
				$(hostname):7:${TABLESPACE_ROOT}/m3/demoDataDir2
				$(hostname):8:${TABLESPACE_ROOT}/m/standby
EOF

    (source "${GPHOME_SOURCE}"/greenplum_path.sh && gpfilespace --config "${TABLESPACE_CONFIG}")

    # create a tablespace in said filespace and a table in that tablespace
    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				CREATE TABLESPACE batsTbsp FILESPACE batsFS;
				CREATE TABLE batsTable(a int) TABLESPACE batsTbsp;
				INSERT INTO batsTable SELECT i from generate_series(1,100)i;
EOF
}

# This is 5X-only
delete_tablespace_data() {
    "${GPHOME_SOURCE}"/bin/psql -d postgres -v ON_ERROR_STOP=1 <<- EOF
				DROP TABLE IF EXISTS batsTable;
				DROP TABLESPACE IF EXISTS batsTbsp;
				DROP FILESPACE IF EXISTS batsFS;
EOF
}

check_tablespace_data() {
    local row_count
    row_count=$("$GPHOME_TARGET"/bin/psql -d postgres -Atc "SELECT COUNT(*) FROM batsTable;")
    if (( row_count != 100 )); then
        fail "failed verifying tablespaces. batsTable got $rows want 100"
    fi
}
