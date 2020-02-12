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

# start_source_cluster() ensures that database is up before returning
start_source_cluster() {
    "${GPHOME}"/bin/pg_isready -q || "${GPHOME}"/bin/gpstart -a
}

# Calls gpdeletesystem on the cluster pointed to by the given master data
# directory.
delete_cluster() {
    local masterdir="$1"

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
    delete_target_datadirs "${masterdir}"
}

delete_finalized_cluster() {
    local masterdir="$1"

    # Sanity check.
    local old_qddir_path=$(dirname $masterdir)"_old/demoDataDir-1"
    if [[ ! -d "$old_qddir_path" ]]; then
        abort "cowardly refusing to delete $masterdir which does not look like an upgraded demo data directory. expected old directory of
            $old_qddir_path"
    fi

    # Look up the master port (fourth line of the postmaster PID file).
    local port=$(awk 'NR == 4 { print $0 }' < "$masterdir/postmaster.pid")

    local gpdeletesystem="$GPHOME_NEW"/bin/gpdeletesystem

    # XXX gpdeletesystem returns 1 if there are warnings. There are always
    # warnings. So we ignore the exit code...
    yes | PGPORT="$port" "$gpdeletesystem" -fd "$masterdir" || true

    # put source directories back into place
    local datadirs=$(dirname "$(dirname "$masterdir")")
    for source_dir in $(find "${datadirs}" -name "*_old"); do
        local new_dirname=$(basename $source_dir _old)
        local new_basedir=$(dirname $source_dir)
        rm -rf "$new_basedir/$new_dirname"
        mv $source_dir "$new_basedir/$new_dirname"
    done
}

delete_target_datadirs() {
    local masterdir="$1"
    local datadir=$(dirname $(dirname "$masterdir"))
    rm -rf "${datadir}"/*_upgrade
}

# require_gnu_stat tries to find a GNU stat program. If one is found, it will be
# assigned to the STAT global variable; otherwise the current test is skipped.
require_gnu_stat() {
    if command -v gstat > /dev/null; then
        STAT=gstat
    elif command -v stat > /dev/null; then
        STAT=stat
    else
        skip "GNU stat is required for this test"
    fi

    # Check to make sure what we have is really GNU.
    local version=$($STAT --version || true)
    [[ $version = *"GNU coreutils"* ]] || skip "GNU stat is required for this test"
}
