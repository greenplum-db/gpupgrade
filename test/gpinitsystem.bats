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

# Takes an old datadir and echoes the expected new datadir path.
#
# NOTE for devs: this is just for getting the expected data directories, which
# is an implementation detail. If you want the actual location of the new master
# data directory after an initialization, you can just ask the hub with
#
#    gpupgrade config show --target-datadir
#
expected_datadir() {
    local base="$(basename $1)"
    local dir="$(dirname $1)_upgrade"

    # Sanity check.
    [ -n "$base" ]
    [ -n "$dir" ]

    echo "$dir/$base"
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
    local newport=6020

    gpupgrade initialize \
        --verbose \
        --source-bindir "$GPHOME/bin" \
        --target-bindir "$GPHOME/bin" \
        --source-master-port "$PGPORT" \
        --temp-port-range 6020-6040 \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    local newmasterdir="$(gpupgrade config show --target-datadir)"
    NEW_CLUSTER="${newmasterdir}"

    # Sanity check the newly created master's location.
    [ "$newmasterdir" = $(expected_datadir "$masterdir") ]

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

        if [ "$newport" = 6021 ]; then
            # This port should be reserved for the standby, which isn't created
            # during initialize. Skip it.
            (( newport++ ))
        fi

        [ -n "$newdir" ] || fail "could not find upgraded segment on expected port $newport"
        [ "$newdir" = $(expected_datadir "$olddir") ]
    done
}

@test "initialize accepts a port range" {
    # We need to have enough ports available for the master, standby, and mirrors.
    # XXX as usual in these tests, we assume a standard demo cluster.
    local expected_ports="15432,15434,15435,15436"
    local mirror_ports="15437,15438,15439"
    local standby_port=15433
    local newport=15432

    gpupgrade initialize \
        --verbose \
        --source-bindir "$GPHOME/bin" \
        --target-bindir "$GPHOME/bin" \
        --source-master-port "$PGPORT" \
        --temp-port-range $expected_ports,$standby_port,$mirror_ports \
        --disk-free-ratio 0 3>&-

    # Make sure we clean up during teardown().
    local newmasterdir="$(gpupgrade config show --target-datadir)"
    NEW_CLUSTER="${newmasterdir}"

    PGPORT=$newport gpstart -a -d "$newmasterdir"

    # save the actual ports
    local actual_ports=$($PSQL -At -p $newport postgres -c "
        select string_agg(port::text, ',' order by content) from gp_segment_configuration
    ")

    # verify ports
    if [ "$expected_ports" != "$actual_ports" ]; then
        fail "want $expected_ports, got $actual_ports"
    fi
}
