#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

if [ "$#" -ne 3 ]; then
    echo "Illegal number of parameters"
    echo "Usage: $(basename $0) <GPHOME> <PGPORT> <OUTPUT_DIR>"
    exit 1
fi

GPHOME=$1
PGPORT=$2
OUTPUT_DIR=$3
APPLY_ONCE_FILES=("gen_alter_gphdfs_roles.sql")

get_databases(){
    databases=$("$GPHOME"/bin/psql -d postgres -p "$PGPORT" -Atc "SELECT datname FROM pg_database WHERE datname != 'template0';")
    echo "$databases"
}

exec_sql_file(){
    local database=$1
    local path=$2
    local file=$3
    local output_file=migration_${database}_${file}

    records=$("$GPHOME"/bin/psql -d "$database" -p "$PGPORT" -Atf "$path")
    if [[ -n "$records" ]]; then
        echo "\c $database" > "${OUTPUT_DIR}/${output_file}"
        echo "$records" >> "${OUTPUT_DIR}/${output_file}"
    fi
}

should_apply_once(){
    local file=$1
    [[ " ${APPLY_ONCE_FILES[*]} " =~ ${file} ]]
}

main(){
    mkdir -p "$OUTPUT_DIR"
    rm -rf "$OUTPUT_DIR"/*.sql

    local databases=($(get_databases))
    local paths=($(find $(dirname "$0") -type f -name "*.sql"))

    for database in "${databases[@]}"; do
        for path in "${paths[@]}"; do
            local file=$(basename "$path")
            # generate sql modifying shared objects only for default database
            if ! should_apply_once "$file" || [ "$database" == "postgres" ]; then
                exec_sql_file "$database" "$path" "$file"
            fi
        done
    done

    echo "Output files are located in: $OUTPUT_DIR"
}

main
