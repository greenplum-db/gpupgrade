#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    gpupgrade initialize \
        --source-bindir="${GPHOME}/bin" \
        --target-bindir="${GPHOME}/bin" \
        --source-master-port="${PGPORT}" \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    rm -r "$STATE_DIR"
}

process_is_running() {
    ps -ef | grep -wGc "$1"
}

@test "kill-services actually stops hub and agents" {
    # check that hub and agent are up
    process_is_running "[g]pupgrade hub"
    process_is_running "[g]pupgrade agent"

    # stop them
    gpupgrade kill-services

    # make sure that they are down
    ! process_is_running "[g]pupgrade hub"
    ! process_is_running "[g]pupgrade agent"
}

@test "kill-services can be run multiple times without issue " {
    gpupgrade kill-services
    gpupgrade kill-services
}

@test "restart-services actually starts hub and agents" {
    gpupgrade kill-services

    # make sure that all services are down
    ! process_is_running "[g]pupgrade hub"
    ! process_is_running "[g]pupgrade agent"

    gpupgrade restart-services

    # check that hub and agent are up
    process_is_running "[g]pupgrade hub"
    process_is_running "[g]pupgrade agent"
}

@test "restart-services can be run even if services are already started" {
    # we rely on the services' being up from setup
    gpupgrade restart-services

    process_is_running "[g]pupgrade hub"
    process_is_running "[g]pupgrade agent"
}
