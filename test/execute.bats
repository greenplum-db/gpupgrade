#! /usr/bin/env bats

load helpers

setup() {
    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    kill_hub
    kill_agents

    # If this variable is set (to a "$port $datadir" pair), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=
}

teardown() {
    kill_hub
    kill_agents
    rm -r "$STATE_DIR"

    if [ -n "$NEW_CLUSTER" ]; then
        # gpdeletesystem returns 1 if there are warnings. There are always
        # warnings. So we ignore the exit code...
        delete_cluster $NEW_CLUSTER || true
    fi
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

# Calls gpdeletesystem. Due to the way gpdeletesystem works, you have to supply
# the master port *and* master data directory. Any arguments after the port are
# considered part of the data directory, to make $NEW_CLUSTER a little easier to
# deal with in the presence of spaces.
delete_cluster() {
    local port=$1; shift
    local masterdir="$*"

    # Sanity check.
    if [[ $masterdir != *_upgrade/demoDataDir* ]]; then
        fail "cowardly refusing to delete $masterdir which does not look like an upgraded demo data directory"
    fi

    local gpdeletesystem="$GPHOME"/bin/gpdeletesystem
    yes | PGPORT="$port" "$gpdeletesystem" -fd "$masterdir"
}

@test "gpupgrade execute runs gpinitsystem based on the source cluster" {
    skip "this test can't work until we fix hub and agent PATH lookup"
    skip_if_no_gpdb

    PSQL="$GPHOME"/bin/psql
    GPSTOP="$GPHOME"/bin/gpstop

    # Store the data directories for each source segment by port.
    run $PSQL -AtF$'\t' -p $PGPORT postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a olddirs
    while read -r port dir; do
        olddirs[$port]="$dir"
    done <<< "$output"
    log "old directories: " $(declare -p olddirs)

    local masterdir="${olddirs[$PGPORT]}"
    local newport=$(( $PGPORT + 1 ))
    local newmasterdir="$(upgrade_datadir $masterdir)"

    # Remove any leftover upgraded cluster.
    # XXX we really need to stop modifying the dev system during a test; can we
    # allow users to override data directories/ports during init-cluster?
    delete_cluster $newport "$newmasterdir" || log "no upgraded cluster running"

    gpupgrade initialize \
        --old-bindir "$GPHOME/bin" \
        --new-bindir "$GPHOME/bin" \
        --old-port "$PGPORT" 3>&-

    gpupgrade execute --verbose

    # Make sure we clean up during teardown().
    NEW_CLUSTER="$newport $newmasterdir"

    # Store the data directories for the new cluster.
    run $PSQL -AtF$'\t' -p $newport postgres -c "select port, datadir from gp_segment_configuration where role = 'p'"
    [ "$status" -eq 0 ] || fail "$output"

    declare -a newdirs
    while read -r port dir; do
        newdirs[$port]="$dir"
    done <<< "$output"
    log "new directories: " $(declare -p newdirs)

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
