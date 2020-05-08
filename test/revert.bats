#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    echo $GPUPGRADE_HOME
}

@test "revert cleans up the state dir when no cluster was created" {
    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --stop-before-cluster-creation \
        --verbose 3>&-

    gpupgrade revert --verbose

    # the GPUPGRADE_HOME directory is deleted
    if [ -d "${GPUPGRADE_HOME}" ]; then
        echo "expected GPUPGRADE_HOME directory ${GPUPGRADE_HOME} to have been deleted"
        exit 1
    fi
}

@test "revert cleans up state dir and data dirs" {
    gpupgrade initialize \
        --source-bindir="$GPHOME/bin" \
        --target-bindir="$GPHOME/bin" \
        --source-master-port="${PGPORT}" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 \
        --verbose 3>&-

    # parse config.json for the datadirs
    local target_hosts_dirs=$(jq -r '.Target.Primaries[] | .Hostname + " " + .DataDir' "${GPUPGRADE_HOME}/config.json")

    # check that the target datadirs exist
    while read -r hostname datadir; do
        ssh "${hostname}" stat "${datadir}" || fail "expected datadir ${datadir} on host ${hostname} to exist"
    done <<< "${target_hosts_dirs}"

    process_is_running "[g]pupgrade hub" || fail 'expected hub to be running'
    process_is_running "[g]pupgrade agent" || fail 'expected agent to be running'

    gpupgrade revert --verbose

    # gpupgrade processes are stopped
    ! process_is_running "[g]pupgrade hub" || fail 'expected hub to have been stopped'
    ! process_is_running "[g]pupgrade agent" || fail 'expected agent to have been stopped'

    # check that the target datadirs were deleted
    while read -r hostname datadir; do
        run ssh "${hostname}" stat "${datadir}"
        ! [ $status -eq 0 ] || fail "expected datadir ${datadir} to have been deleted"
    done <<< "${target_hosts_dirs}"

    # the GPUPGRADE_HOME directory is deleted
    if [ -d "${GPUPGRADE_HOME}" ]; then
        echo "expected GPUPGRADE_HOME directory ${GPUPGRADE_HOME} to have been deleted"
        exit 1
    fi

    # check that archive directory has been created
    [ -d "$HOME/gpAdminLogs/gpupgrade-"* ] || fail "expected directory matching $HOME/gpAdminLogs/gpupgrade-* to be created"
}
