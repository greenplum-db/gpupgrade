#!/usr/bin/env bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
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

After running gpupgrade-migration-sql-generator.bash, run gpupgrade-migration-sql-executor.bash.
Run gpupgrade-migration-sql-executor.bash -h for more information.'
}

if [ "$#" -eq 0 ] || ([ "$#" -eq 1 ] && ([ "$1" = -h ] || [ "$1" = --help ])) ; then
    print_usage
    exit 0
fi

if ! { [ "$#" -eq 3 ] || [ "$#" -eq 4 ]; } ; then
    echo ""
    echo "Error: Incorrect number of arguments"
    print_usage
    exit 1
fi

GPHOME=$1
PGPORT=$2
OUTPUT_DIR=$3
# INPUT_DIR is a hidden option used for internal testing or support
INPUT_DIR=${4:-"$(dirname "$0")/greenplum/gpupgrade/data-migration-scripts"}
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
        records=$("$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atf "$path")
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

    local paths=($(find "${INPUT_DIR}/${dir}" -type f \( -name "*.sql" -o -name "*.sh" \) | sort -n))
    local output_dir="${OUTPUT_DIR}/${dir}"

    mkdir -p "$output_dir"
    rm -f "$output_dir"/*.sql
    rm -f "$output_dir"/*.sh

    for database in "${databases[@]}"; do
        # Create the function for detecting dependent views
        is_python_enabled=$("$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atc "SELECT count(*) FROM pg_language WHERE lanname = 'plpythonu'")
        if [ ! $is_python_enabled == 1 ]; then
            is_python_enabled=$("$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atc "CREATE LANGUAGE plpythonu")
        fi
        records=$("$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atf "${INPUT_DIR}/create_find_view_dep_function.sql")

        for path in "${paths[@]}"; do
            # generate sql modifying shared objects only for default database
            if ! should_apply_once "$path" || [ "$database" == "postgres" ]; then
                exec_script "$database" "$path" "$output_dir"
            fi
        done

        # Drop the table of dependent views
        records=$(PGOPTIONS='--client-min-messages=warning' \
            "$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atc \
            "DROP TABLE IF EXISTS __gpupgrade_tmp.__temp_views_list")

        # Drop the temp schema
        records=$(PGOPTIONS='--client-min-messages=warning' \
            "$GPHOME"/bin/psql -X -q -d "$database" -p "$PGPORT" -Atc \
            "DROP SCHEMA IF EXISTS __gpupgrade_tmp")
    done

    echo "Output files are located in: $output_dir"
}

main(){
    local dirs=(pre-initialize post-finalize post-revert stats)
    local databases=($(get_databases))

    local current_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local user_input
    for dir in "${dirs[@]}"; do
        if [ -d "${OUTPUT_DIR}/${dir}" ]; then
            # if user already provided an input, don't ask again.
            if [[ -z $user_input ]]; then
                read -rp "Directories exists from a previous run, press 'Y' to archive them under ${OUTPUT_DIR}/archive and continue or 'N' to exit: " user_input
            fi

            if [[ "$user_input" == "Y" || "$user_input" == "y" ]]; then
                mkdir -p "${OUTPUT_DIR}/archive"
                mv "${OUTPUT_DIR}/${dir}" "${OUTPUT_DIR}/archive/${dir}_${current_timestamp}"
            else
                echo "You entered $user_input, exiting!"
                exit 1
            fi
        fi
    done

    for dir in "${dirs[@]}"; do
        execute_script_directory "${dir}" "${databases[@]}"
    done
}

main
