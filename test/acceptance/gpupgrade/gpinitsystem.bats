#! /usr/bin/env bats
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load ../helpers/helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    # If this variable is set (to a coordinator data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=

    PSQL="$GPHOME_SOURCE"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    archive_state_dir "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $GPHOME_TARGET $NEW_CLUSTER
    fi
}

@test "initialize runs gpinitsystem based on the source cluster" {
    # Store the data directories for each source segment by port.
    run get_segment_configuration "$GPHOME_SOURCE"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a olddirs
    while read -r content role hostname port datadir; do
        if [ "$role" == "p" ]; then
            olddirs[$port]="$datadir"
        fi
    done <<< "$output"

    local coordinator_dir="${olddirs[$PGPORT]}"
    local newport=6020

    gpupgrade initialize \
        --automatic \
        --verbose \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "$PGPORT" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    local new_coordinator_dir="$(gpupgrade config show --target-datadir)"
    NEW_CLUSTER="${new_coordinator_dir}"

    # Sanity check the newly created coordinator's location.
    [ "$new_coordinator_dir" = $(expected_target_datadir "$coordinator_dir") ]

    # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    (unset LD_LIBRARY_PATH; PGPORT=$newport source "$GPHOME_TARGET"/greenplum_path.sh && gpstart -a -d "$new_coordinator_dir")

    # Store the data directories for the new cluster.
    run get_segment_configuration "$GPHOME_TARGET" "$newport"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a newdirs
    while read -r content role hostname port datadir; do
        if [ "$role" == "p" ]; then
            newdirs[$port]="$datadir"
        fi
    done <<< "$output"

    # Ensure the new cluster has the expected ports and compare the directories
    # between the two clusters. We assume the new ports are assigned in
    # ascending order of content ids.
    for olddir in "${olddirs[@]}"; do
        local newdir="${newdirs[$newport]}"
        (( newport++ ))

        if [ "$newport" = 6021 ]; then
            # This port should be reserved for the standby, which isn't created
            # during initialize. Skip it.
            (( newport++ ))
        fi

        [ -n "$newdir" ] || fail "could not find upgraded segment on expected port $newport"
        [ "$newdir" = $(expected_target_datadir "$olddir") ]
    done
}

@test "initialize accepts a port range" {
    # We need to have enough ports available for the coordinator, standby, and
    # mirrors. As usual in these tests, we assume a standard demo cluster.
    # XXX: GPDB 5 demo cluster uses port 15432 by default so pick ports
    # not in the ephemeral range that do not conflict with it.
    local expected_ports="30432,30434,30435,30436"
    local mirror_ports="30437,30438,30439"
    local standby_port=30433
    local newport=30432

    gpupgrade initialize \
        --automatic \
        --verbose \
        --source-gphome "$GPHOME_SOURCE" \
        --target-gphome "$GPHOME_TARGET" \
        --source-master-port "$PGPORT" \
        --temp-port-range $expected_ports,$standby_port,$mirror_ports \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    local new_coordinator_dir="$(gpupgrade config show --target-datadir)"
    NEW_CLUSTER="${new_coordinator_dir}"

    # unset LD_LIBRARY_PATH due to https://web.archive.org/web/20220506055918/https://groups.google.com/a/greenplum.org/g/gpdb-dev/c/JN-YwjCCReY/m/0L9wBOvlAQAJ
    (unset LD_LIBRARY_PATH; PGPORT=$newport source "$GPHOME_TARGET"/greenplum_path.sh && gpstart -a -d "$new_coordinator_dir")

    # save the actual ports
    local actual_ports=$($PSQL -At -p $newport postgres -c "
        select string_agg(port::text, ',' order by content) from gp_segment_configuration
    ")

    # verify ports
    if [ "$expected_ports" != "$actual_ports" ]; then
        fail "want $expected_ports, got $actual_ports"
    fi
}
