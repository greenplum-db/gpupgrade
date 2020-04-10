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

    # NOTE: the target master datadir now looks something like this: qddir/demoDataDir.k9KuElo8HT8.-1

    # Sanity check.
    if [[ $masterdir != *qddir/demoDataDir*\.*\.-1* ]]; then
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

# this is a bash version of the golang upgrade.ArchiveDirectoryForSource() function
archiveDataDir() {
    local dataDir="$1"
    local upgradeID="$2"

    local baseDir
    baseDir=$(basename "$dataDir")
    local dirPath
    dirPath=$(dirname "$dataDir")

    echo "${dirPath}/${baseDir}.${upgradeID}.old"

}

# after the end of a succesful upgrade, we have:
#   source dataDirs --> archive dataDirs
#   target dataDirs --> (in location of original source dataDirs)
# This function deletes the target cluster and replaces the source dataDirs from their archive locations
delete_finalized_cluster() {
    local masterdir="$1"
    local upgradeID="$2"

    # Sanity check.
    local archive_qddir_path
    archive_qddir_path=$(archiveDataDir "$masterdir" "$upgradeID")
    if [[ ! -d "$archive_qddir_path" ]]; then
        abort "cowardly refusing to delete ${masterdir} which does not look like an upgraded demo data directory. expected old directory of
            $archive_qddir_path"
    fi

    # Look up the master port (fourth line of the postmaster PID file).
    local port=$(awk 'NR == 4 { print $0 }' < "$masterdir/postmaster.pid")

    local gpdeletesystem="$GPHOME"/bin/gpdeletesystem

    # XXX gpdeletesystem returns 1 if there are warnings. There are always
    # warnings. So we ignore the exit code...
    yes | PGPORT="$port" "$gpdeletesystem" -fd "$masterdir" || true

    # put source directories back into place
    local datadirs
    datadirs=$(dirname "$(dirname "$masterdir")")

    local nonStandbyAchiveDirs
    nonStandbyAchiveDirs=$(ls -d  ${datadirs}/*/*.${upgradeID}.old)
    local standbyArchiveDir
    standbyArchiveDir=$(ls -d ${datadirs}/standby.${upgradeID}.old)
    local archiveDirs
    archiveDirs=(${nonStandbyAchiveDirs[@]} "${standbyArchiveDir}")

    for archiveDir in "${archiveDirs[@]}"; do
        local new_dirname
        new_dirname=$(basename "$archiveDir" ".${upgradeID}.old")
        local new_basedir
        new_basedir=$(dirname "$archiveDir")
        rm -rf "${new_basedir:?}/${new_dirname}"
        mv "${archiveDir}" "${new_basedir}/${new_dirname}"
    done

}

delete_target_datadirs() {
    local masterdir="$1"
    local datadir=$(dirname "$(dirname "$masterdir")")

    rm -rf "${datadir}"/*/demoDataDir.*.[0-9]
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
