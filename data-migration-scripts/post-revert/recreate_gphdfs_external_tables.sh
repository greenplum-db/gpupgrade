#!/bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

if [ "$#" -ne 3 ]; then
    echo "Illegal number of parameters"
    echo "Usage: $(basename "$0") <GPHOME> <PGPORT> <DBNAME>"
    exit 1
fi

GPHOME=$1
PGPORT=$2
DBNAME=$3

main() {
    local psql="$GPHOME"/bin/psql
    local pg_dump="$GPHOME"/bin/pg_dump

    local tables=()

    # Find all GPHDFS external tables.
    #
    # NOTE: psql's -c implies -X; we don't need to worry about .psqlrc
    # influencing these queries. For 8.3 this is undocumented but still true.
    while read -r table; do
        tables+=(-t "$table")
    done < <($psql -d "$DBNAME" -p "$PGPORT" -Atc "
        SELECT d.objid::regclass
        FROM pg_catalog.pg_depend d
               JOIN pg_catalog.pg_exttable x ON ( d.objid = x.reloid )
               JOIN pg_catalog.pg_extprotocol p ON ( p.oid = d.refobjid )
               JOIN pg_catalog.pg_class c ON ( c.oid = d.objid )
        WHERE d.refclassid = 'pg_extprotocol'::regclass
            AND p.ptcname = 'gphdfs';
    ")

    if (( ! ${#tables[@]} )); then
        # Don't pg_dump if there are no interesting tables; we'll get useless
        # SQL files.
        return
    fi

    $pg_dump -p "$PGPORT" --schema-only "${tables[@]}" "$DBNAME"
}

main
