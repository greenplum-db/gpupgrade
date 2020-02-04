#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=

    PSQL="$GPHOME"/bin/psql
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    rm -r "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $NEW_CLUSTER
    fi
}

@test "initialize runs gpinitsystem based on the source cluster" {
    # Store the data directories for each source segment by port.
    run $PSQL -AtF$'\t' -p $PGPORT postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a olddirs
    while read -r port dir; do
        olddirs[$port]="$dir"
    done <<< "$output"

    local masterdir="${olddirs[$PGPORT]}"
    local newport=50432
    local newmasterdir="$(upgrade_datadir $masterdir)"

    gpupgrade initialize \
        --verbose \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    NEW_CLUSTER="$newmasterdir"

    PGPORT=$newport gpstart -a -d "$newmasterdir"

    # Store the data directories for the new cluster.
    run $PSQL -AtF$'\t' -p $newport postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a newdirs
    while read -r port dir; do
        newdirs[$port]="$dir"
    done <<< "$output"

    # Ensure the new cluster has the expected ports and compare the directories
    # between the two clusters. We assume the new ports are assigned in
    # ascending order of content ids.
    for olddir in "${olddirs[@]}"; do
        local newdir="${newdirs[$newport]}"
        (( newport++ ))

        [ -n "$newdir" ] || fail "could not find upgraded segment on expected port $newport"
        [ "$newdir" = $(upgrade_datadir "$olddir") ]
    done
}

@test "initialize accepts a port range" {
    local expected_ports="15432,15433,15434,15435"
    local newport=15432

    local masterdir="$($PSQL -At postgres -c "select datadir from gp_segment_configuration where content = -1 and role = 'p'")"
    local newmasterdir="$(upgrade_datadir $masterdir)"

    gpupgrade initialize \
        --ports $expected_ports \
        --verbose \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    NEW_CLUSTER="$newmasterdir"

    PGPORT=$newport gpstart -a -d "$newmasterdir"

    # save the actual ports
    local actual_ports=$($PSQL -At -p $newport postgres -c "select string_agg(port::text, ',' order by content) from gp_segment_configuration")

    # verify ports
    if [ "$expected_ports" != "$actual_ports" ]; then
        fail "want $expected_ports, got $actual_ports"
    fi
}
