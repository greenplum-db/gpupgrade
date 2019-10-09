# log() prints its arguments to the TAP stream. Newlines are supported (each
# line will be correctly escaped in TAP).
log() {
    while read -r line; do
        echo "# $line" 1>&3
    done <<< "$*"
}

# fail() is meant to be called from BATS tests. It will fail the current test
# after printing its arguments to the TAP stream.
fail() {
    log "$@"
    false
}

# abort() is meant to be called from BATS tests. It will exit the process after
# printing its arguments to the TAP stream.
abort() {
    log "fatal: $*"
    exit 1
}

# skip_if_no_gpdb() will skip a test if a cluster's environment is not set up.
skip_if_no_gpdb() {
    [ -n "${GPHOME}" ] || skip "this test requires an active GPDB cluster (set GPHOME)"
    [ -n "${PGPORT}" ] || skip "this test requires an active GPDB cluster (set PGPORT)"
}

# Calls gpdeletesystem on the cluster pointed to by the given master data
# directory.
delete_cluster() {
    local masterdir="$1"
    local datadir=$(dirname $(dirname "$masterdir"))

    # Sanity check.
    if [[ $masterdir != *_upgrade/demoDataDir* ]]; then
        abort "cowardly refusing to delete $masterdir which does not look like an upgraded demo data directory"
    fi

    # Look up the master port (fourth line of the postmaster PID file).
    local port=$(awk 'NR == 4 { print $0 }' < "$masterdir/postmaster.pid")

    local gpdeletesystem="$GPHOME"/bin/gpdeletesystem

    # XXX gpdeletesystem returns 1 if there are warnings. There are always
    # warnings. So we ignore the exit code...
    yes | PGPORT="$port" "$gpdeletesystem" -fd "$masterdir" || true

    # XXX The master datadir copy moves the datadirs to .old instead of
    # removing them. This causes gpupgrade to fail when copying the master
    # data directory to segments with "file exists".
    rm -rf "${datadir}"/*_upgrade
}
