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
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    rm -r "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        delete_cluster $NEW_CLUSTER
    fi

    gpstart -a
}

# Takes an old datadir and echoes the expected new datadir path.
upgrade_datadir() {
    local base="$(basename $1)"
    local dir="$(dirname $1)_upgrade"

    # Sanity check.
    [ -n "$base" ]
    [ -n "$dir" ]

    echo "$dir/$base"
}

@test "gpupgrade execute runs gpinitsystem based on the source cluster" {
    skip_if_no_gpdb

    PSQL="$GPHOME"/bin/psql

    # Store the data directories for each source segment by port.
    run $PSQL -AtF$'\t' -p $PGPORT postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a olddirs
    while read -r port dir; do
        olddirs[$port]="$dir"
    done <<< "$output"

    local masterdir="${olddirs[$PGPORT]}"
    local newport=$(( $PGPORT + 1 ))
    local newmasterdir="$(upgrade_datadir $masterdir)"

    gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" 3>&-

    gpupgrade execute

    # Make sure we clean up during teardown().
    NEW_CLUSTER="$newmasterdir"

    # Store the data directories for the new cluster.
    run $PSQL -AtF$'\t' -p $newport postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a newdirs
    while read -r port dir; do
        newdirs[$port]="$dir"
    done <<< "$output"

    # Compare the ports and directories between the two clusters.
    for port in "${!olddirs[@]}"; do
        local olddir="${olddirs[$port]}"
        local newdir

        # Master is special -- the new master is only incremented by one.
        # Primary ports are incremented by 4000.
        if [ $port -eq $PGPORT ]; then
            (( newport = $port + 1 ))
        else
            (( newport = $port + 4000 ))
        fi
        newdir="${newdirs[$newport]}"

        [ -n "$newdir" ] || fail "could not find upgraded primary on expected port $newport"
        [ "$newdir" = $(upgrade_datadir "$olddir") ]
    done
}
