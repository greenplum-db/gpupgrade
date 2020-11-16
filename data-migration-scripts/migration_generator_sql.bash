#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

function print_usage() {
echo '
Identifies catalog inconsistencies between the source and target Greenplum versions
and generates SQL scripts to resolve them. This command should be run prior to "gpupgrade".

Usage: '$(basename $0)' <GPHOME> <PGPORT> <OUTPUT_DIR>
     <GPHOME>     : the path to the source Greenplum installation directory
     <PGPORT>     : the source Greenplum system port number
     <OUTPUT_DIR> : the user-defined directory where the SQL scripts are created

The output directory structure is:
     <output directory>
     + pre-initialize  drop and alter objects prior to "gpupgrade initialize"
     + post-finalize   restore and recreate objects following "gpupgrade finalize"
     + post-revert     restore objects following "gpupgrade revert"

After running migration_generator_sql.bash, run migration_executor_sql.bash.
Run migration_executor_sql.bash -h for more information.'
}

if [ "$#" -eq 0 ] || ([ "$#" -eq 1 ] && ([ "$1" = -h ] || [ "$1" = --help ])) ; then
    print_usage
    exit 0
fi

if [ "$#" -ne 3 ]; then
    echo ""
    echo "Error: Incorrect number of arguments"
    print_usage
    exit 1
fi

GPHOME=$1
PGPORT=$2
OUTPUT_DIR=$3
APPLY_ONCE_FILES=("gen_alter_gphdfs_roles.sql")

get_databases(){
    databases=$("$GPHOME"/bin/psql -X -d postgres -p "$PGPORT" -Atc "SELECT datname FROM pg_database WHERE datname != 'template0';")
    echo "$databases"
}

exec_script(){
    local database=$1
    local path=$2
    local output_dir=$3

    local name
    name=$(basename "$path")
    name="${name%.*}" # strip extensions

    local output_file=migration_${database}_${name}.sql

    local records
    if [[ $path == *".sql" ]]; then
        records=$("$GPHOME"/bin/psql -X -d "$database" -p "$PGPORT" -Atf "$path")
    else
        records=$("$path" "$GPHOME" "$PGPORT" "$database")
    fi

    if [[ -n "$records" ]]; then
        # change database before header, to allow header to define SQL functions
        echo "\c $database" >> "${output_dir}/${output_file}"

        local basename suffix header_file
        basename=$(basename "$path")
        suffix=${basename##*.}
        if [[ "$suffix" == "$basename" ]]; then
          header_file="${path}.header"
        else
          header_file=${path/%.$suffix/.header}
        fi
        if [[ -f $header_file ]]; then
            cat "$header_file" >> "${output_dir}/${output_file}"
        fi

        echo "$records" >> "${output_dir}/${output_file}"
    fi
}

should_apply_once(){
    local path=$1
    local file=$(basename "$path")
    [[ " ${APPLY_ONCE_FILES[*]} " =~ ${file} ]]
}

execute_script_directory() {
    local dir=$1; shift
    local databases=( "$@" )

    local paths=($(find "$(dirname "$0")/${dir}" -type f \( -name "*.sql" -o -name "*.sh" \) | sort -n))
    local output_dir="${OUTPUT_DIR}/${dir}"

    mkdir -p "$output_dir"
    rm -f "$output_dir"/*.sql
    rm -f "$output_dir"/*.sh

    for database in "${databases[@]}"; do
        for path in "${paths[@]}"; do
            # generate sql modifying shared objects only for default database
            if ! should_apply_once "$path" || [ "$database" == "postgres" ]; then
                exec_script "$database" "$path" "$output_dir"
            fi
        done
    done

    echo "Output files are located in: $output_dir"
}

main(){
    local dirs=(pre-initialize post-finalize post-revert stats)
    local databases=($(get_databases))

    for dir in "${dirs[@]}"; do
        execute_script_directory "$dir" "${databases[@]}"
    done
}

main
