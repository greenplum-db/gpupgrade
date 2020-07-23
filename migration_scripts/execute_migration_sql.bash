#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

if [ "$#" -ne 3 ]; then
    echo "Illegal number of parameters"
    echo "Usage: $(basename $0) <GPHOME> <PGPORT> <INPUT_DIR>"
    exit 1
fi

GPHOME=$1
PGPORT=$2
INPUT_DIR=$3

main(){
    local log_file="${INPUT_DIR}/migration_sql.log"

    rm -f "$log_file"

    cmd="find ${INPUT_DIR} -type f -name *.sql | sort -n"
    local files="$(eval "$cmd")"
    if [ -z "$files" ]; then
        echo "Executing command \"${cmd}\" returned no sql files. Exiting!" | tee -a "$log_file"
        exit 1
    fi

    for file in ${files[*]}; do
        local cmd="${GPHOME}/bin/psql -d postgres -p ${PGPORT} -f ${file}"
        echo "Executing command: ${cmd}" | tee -a "$log_file"
        ${cmd} | tee -a "$log_file"
    done

    echo "Check log file for execution details: $log_file"
}

main
