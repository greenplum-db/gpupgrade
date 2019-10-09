#! /usr/bin/env bats

load helpers

setup() {
    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services
}

teardown() {
    gpupgrade kill-services
    rm -r "$STATE_DIR"
}

# Prints the number of primaries in the cluster.
get_primary_count() {
    local port=$1

    local psql="$GPHOME"/bin/psql
    $psql -At -p $port postgres -c '
        SELECT COUNT(DISTINCT content) FROM gp_segment_configuration
            WHERE content > -1
    '
}

@test "conversion status is displayed for all segments" {
    skip_if_no_gpdb

    # XXX It would be nice if we didn't have to initialize just to get a hub
    # that could respond to RPC.
    gpupgrade initialize \
        --old-bindir=$PWD \
        --new-bindir=$PWD \
        --old-port=$PGPORT 3>&-

    run gpupgrade status conversion
    [ "$status" -eq 0 ] || fail "$output"

    local count=$(get_primary_count $PGPORT)
    for (( i = 0; i < $count; i++ )); do
        local pattern="PENDING - DBID ? - CONTENT ID $i - PRIMARY -*"
        if ! [[ ${lines[$i]} = $pattern ]]; then
            log "needed '$pattern'; actual follows"
            fail "$output"
        fi
    done
}
