#! /usr/bin/env bats

load test/helpers

setup() {
    [ ! -z $GPHOME ]
    [ ! -z $MASTER_DATA_DIRECTORY ]
    echo "# SETUP" 1>&3
    clean_target_cluster
    clean_statedir
    kill_hub
    kill_agents
}

@test "init-cluster can successfully create the target cluster and retrieve its configuration" {
    gpupgrade prepare init \
              --new-bindir "$GPHOME"/bin \
              --old-bindir "$GPHOME"/bin

    gpupgrade prepare start-hub 3>&-

    gpupgrade check config
    gpupgrade check version
    gpupgrade check seginstall

    gpupgrade prepare start-agents
    sleep 1

    run gpupgrade prepare init-cluster
    [ "$status" -eq 0 ]

    echo "# Waiting for init to complete" 1>&3
    local observed_complete="false"
    for i in {1..60}; do
        echo "## checking status ($i/60)" 1>&3
        run gpupgrade status upgrade
        [ "$status" -eq 0 ]
        [[ "$output" != *"FAILED"* ]]

        if [[ "$output" = *"COMPLETE - Initialize upgrade target cluster"* ]]; then
            observed_complete="true"
            break
        fi

        sleep "$i"
    done

    [ "$observed_complete" != "false" ]

    #TODO: gpupgrade prepare shutdown-clusters
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
