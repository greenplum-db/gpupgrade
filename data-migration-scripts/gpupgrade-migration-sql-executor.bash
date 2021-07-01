#!/usr/bin/env bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

function print_usage() {
echo '
Executes scripts addressing catalog inconsistencies between Greenplum versions.
Before running this command, run gpupgrade-migration-sql-generator.bash.
This command should be run only during the downtime window.

Usage: '$(basename $0)' <GPHOME> <PGPORT> <INPUT_DIR>
     <GPHOME>    : the path to the source Greenplum installation directory
     <PGPORT>    : the source Greenplum system port number
     <INPUT_DIR> : the directory containing the scripts to execute. This is the
                   <OUTPUT_DIR> you specified earlier in gpupgrade-migration-sql-generator.bash,
                   with a subdirectory appended as in the use cases below. The
                   subdirectories are pre-initialize, post-finalize, and post-revert.

Use cases:
- Before "gpupgrade initialize", drop and alter objects:
gpupgrade-migration-sql-executor.bash /path/to/gphome 5432 /path/to/output_dir/pre-initialize

- Following "gpupgrade finalize", restore and recreate objects:
gpupgrade-migration-sql-executor.bash /path/to/gphome 5432 /path/to/output_dir/post-finalize

- Following "gpupgrade revert", restore objects:
gpupgrade-migration-sql-executor.bash /path/to/gphome 5432 /path/to/output_dir/post-revert

Log files can be found in INPUT_DIR/data_migration.log'
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
INPUT_DIR=$3

main(){
    local log_file="${INPUT_DIR}/data_migration.log"

    rm -f "$log_file"

    cmd="find ${INPUT_DIR} -type f -name \"*.sql\" | sort -n"
    local files="$(eval "$cmd")"
    if [ -z "$files" ]; then
        echo "Executing command \"${cmd}\" returned no sql files. Exiting!" | tee -a "$log_file"
        exit 1
    fi

    for file in ${files[*]}; do
        local cmd="${GPHOME}/bin/psql -X -d postgres -p ${PGPORT} -f ${file} --echo-queries --quiet"
        echo "Executing command: ${cmd}" | tee -a "$log_file"
        ${cmd} 2>&1 | tee -a "$log_file"
    done

    echo "Check log file for execution details: $log_file"
}

main
