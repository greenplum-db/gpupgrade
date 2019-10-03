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
    kill_hub
    kill_agents
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
              --old-port 15432 3>&-

    gpupgrade execute
    gpupgrade finalize

    run gpupgrade status upgrade
    [ "$status" -eq 0 ]
    # TODO: fix the PENDING step to go to the COMPLETE state.
    if ! [[
            "${lines[0]}" = *"COMPLETE - Configuration Check"* &&
            "${lines[1]}" = *"COMPLETE - Agents Started on Cluster"* &&
            "${lines[2]}" = *"COMPLETE - Initialize new cluster"* &&
            "${lines[3]}" = *"COMPLETE - Shutdown clusters"* &&
            "${lines[4]}" = *"COMPLETE - Run pg_upgrade on master"* &&
            "${lines[5]}" = *"COMPLETE - Copy master data directory to segments"* &&
            "${lines[6]}" = *"PENDING - Run pg_upgrade on primaries"* &&
            "${lines[7]}" = *"COMPLETE - Validate the upgraded cluster can start up"* &&
            "${lines[8]}" = *"COMPLETE - Adjust upgraded cluster ports"*
         ]]; then
        fail "actual: $output"
    fi
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
