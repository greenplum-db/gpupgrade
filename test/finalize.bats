#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
    gpupgrade kill-services

    # If this variable is set (to a master data directory), teardown() will call
    # gpdeletesystem on this cluster.
    NEW_CLUSTER=

    # Store the ports in use on the cluster.
    OLD_PORTS=$(get_ports)

    # Set up an upgrade based on the live cluster, then stop the cluster (to
    # mimic an actual upgrade).
    gpupgrade initialize \
        --old-bindir="$GPHOME/bin" \
        --new-bindir="$GPHOME/bin" \
        --old-port=$PGPORT  \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
    gpstop -a
}

teardown() {
    # XXX Beware, BATS_TEST_SKIPPED is not a documented export.
    if [ -z "${BATS_TEST_SKIPPED}" ]; then
        gpupgrade kill-services

        if [ -n "$NEW_CLUSTER" ]; then
            delete_cluster $NEW_CLUSTER
        fi
        rm -rf "$STATE_DIR/demoDataDir*"
        rm -r "$STATE_DIR"

        start_source_cluster
    fi
}

@test "finalize modifies ports on the live target cluster" {

    # To avoid spinning up an entire upgrade just to test finalize, we instead
    # create a new cluster for the test and fake the configurations to point at
    # it.
    #
    # XXX we assume three primaries (demo cluster layout)
    # XXX we hardcode ports here, so we'll fail if there are any conflicts.
    mkdir "$STATE_DIR/_upgrade"
    echo localhost > "$STATE_DIR/hostfile"
    cat - > "$STATE_DIR/gpinitsystem_config" <<EOF
ARRAY_NAME="gpupgrade test cluster"
MASTER_HOSTNAME=localhost
MACHINE_LIST_FILE="$STATE_DIR/hostfile"

MASTER_PORT=40000
PORT_BASE=50000

SEG_PREFIX=demoDataDir
MASTER_DIRECTORY="$STATE_DIR/_upgrade"
declare -a DATA_DIRECTORY=("$STATE_DIR/_upgrade" "$STATE_DIR/_upgrade" "$STATE_DIR/_upgrade")

TRUSTED_SHELL=ssh
CHECK_POINT_SEGMENTS=8
ENCODING=UNICODE
EOF

    # XXX There are always warnings, so ignore them...
    gpinitsystem -ac "$STATE_DIR/gpinitsystem_config" 3>&- || true
    NEW_CLUSTER="$STATE_DIR/_upgrade/demoDataDir-1"

    # Generate a new target cluster configuration that the hub can use, then
    # restart the hub.
    PGPORT=40000 go run ./testutils/insert_target_config "$GPHOME/bin" "$GPUPGRADE_HOME/config.json"
    gpupgrade kill-services
    gpupgrade hub --daemonize 3>&-

    gpupgrade finalize

    # Sanity check: make sure the "new cluster" is really the new cluster by
    # verifying the master data directory location.
    datadir=$(psql -At postgres -c "select datadir from gp_segment_configuration
                                    where content = -1 and role = 'p'")
    [ "$datadir" = "$STATE_DIR/_upgrade/demoDataDir-1" ] || fail "actual master datadir: $datadir"

    # Check to make sure the new cluster's ports match the old one.
    local new_ports=$(get_ports)
    [ "$OLD_PORTS" = "$new_ports" ] || fail "actual ports: $new_ports"
}

# Writes the primary ports from the cluster pointed to by $PGPORT to stdout, one
# per line, sorted by content ID.
get_ports() {
    PSQL="$GPHOME"/bin/psql
    $PSQL -At postgres \
        -c "select port from gp_segment_configuration where role = 'p' order by content"
}
