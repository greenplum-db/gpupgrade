#! /usr/bin/env bats

load test/helpers

# If GPHOME_NEW is not set, then it defaults to GPHOME, doing a upgrade to the
#  samve version

setup() {
    [ ! -z $GPHOME ]
    GPHOME_NEW=${GPHOME_NEW:-$GPHOME}
    [ ! -z $MASTER_DATA_DIRECTORY ]
    echo "# SETUP"
    clean_target_cluster
    clean_statedir
    gpupgrade kill-services
}

teardown() {
    echo "# TEARDOWN"
    if ! psql -d postgres -c ''; then
        gpstart -a
    fi
}

@test "gpugrade can make it as far as we currently know..." {
    gpupgrade initialize \
              --old-bindir "$GPHOME"/bin \
              --new-bindir "$GPHOME_NEW"/bin \
              --source-master-port $PGPORT \
              --disk-free-ratio=0 \
              3>&-

    gpupgrade execute
    gpupgrade finalize
}

clean_target_cluster() {
    ps -ef | grep postgres | grep _upgrade | awk '{print $2}' | xargs kill || true
    rm -rf "$MASTER_DATA_DIRECTORY"/../../*_upgrade
    # TODO: Can we be less sketchy ^^
    # gpdeletesystem -d "$MASTER_DATA_DIRECTORY"/../../*_upgrade #FORCE?
}

clean_statedir() {
  rm -rf ~/.gpupgrade
  rm -rf ~/gpAdminLogs/
}
