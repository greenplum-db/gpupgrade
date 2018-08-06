#! /usr/bin/env bats

load test/helpers

setup() {
    [ ! -z $GPHOME ]
    [ ! -z $MASTER_DATA_DIRECTORY ]
    echo "# SETUP" 1>&3
    # XXX: We are assuming that the source cluster comes from the gpdb demo
    # cluster. It would be good to challenge this, but with an example workflow.
    clean_target_cluster
    clean_statedir
    kill_hub
    kill_agents
    run gpstart -a
}

# XXX: For now, leave system alone the end and push all cleanup into the setup().

@test "init-cluster can successfully create the target cluster and retrieve its configuration" {
    gpupgrade prepare init \
              --new-bindir "$GPHOME"/bin \
              --old-bindir "$GPHOME"/bin

    gpupgrade prepare start-hub
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
    ps -ef | grep postgres | grep _upgrade | awk '{print "/tmp/.s.PGSQL."$12".lock"}' | xargs -0 -I {} rm -f {}
    ps -ef | grep postgres | grep _upgrade | awk '{print "/tmp/.s.PGSQL."$12}' | xargs -0 -I {} rm -f {}

    # Apparently we can't be clever enough ^-- so dumb works --v
    TARGET_MASTER_PORT=$((PGPORT+1))
    rm -f /tmp/.s.PGSQL."$TARGET_MASTER_PORT".lock

    ps -ef | grep postgres | grep _upgrade | awk '{print $2}' | xargs kill -9 || true

    rm -rf "$MASTER_DATA_DIRECTORY"/../../*_upgrade
}

clean_statedir() {
  rm -rf ~/.gpupgrade
  rm -rf ~/gpAdminLogs/
}
