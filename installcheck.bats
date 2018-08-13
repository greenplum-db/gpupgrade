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

teardown() {
    if ! psql -d postgres -c ''; then
        gpstart -a
    fi
}

@test "gpugrade can make it as far as we currently know..." {
    gpupgrade prepare init \
              --new-bindir "$GPHOME"/bin \
              --old-bindir "$GPHOME"/bin

    gpupgrade prepare start-hub 3>&-

    gpupgrade check config
    gpupgrade check version
    gpupgrade check seginstall

    gpupgrade prepare start-agents
    sleep 1

    gpupgrade prepare init-cluster

    EventuallyStepCompletes "Initialize upgrade target cluster"

    gpupgrade prepare shutdown-clusters

    EventuallyStepCompletes "Shutdown clusters"

    ! ps -ef | grep -Gqw "[p]ostgres"

    gpupgrade upgrade convert-master

    EventuallyStepCompletes "Run pg_upgrade on master"
}

EventuallyStepCompletes() {
    cliStepMessage="$1"
    echo "# Waiting for \"$cliStepMessage\" to transition to complete" 1>&3
    local observed_complete="false"
    for i in {1..60}; do
        run gpupgrade status upgrade
        [ "$status" -eq 0 ]

        statusLine=$(echo "$output" | grep "$cliStepMessage")
        echo "# $statusLine ($i/60)" 1>&3

        if [[ "$statusLine" = *"FAILED"* ]]; then
            break
        fi


        if [[ "$output" = *"COMPLETE - $cliStepMessage"* ]]; then
            observed_complete="true"
            break
        fi

        sleep "$i"
    done

    [ "$observed_complete" != "false" ]
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
