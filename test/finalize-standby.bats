#!/usr/bin/env bats

load helpers

setup_state_dir() {
    STATE_DIR=$(mktemp -d /tmp/gpupgrade.XXXXXX)
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

teardown_new_cluster() {
    delete_finalized_cluster $MASTER_DATA_DIRECTORY
}

setup() {
    skip_if_no_gpdb

    [ ! -z $GPHOME ]
    GPHOME_NEW=${GPHOME_NEW:-$GPHOME}

    setup_state_dir

    gpupgrade kill-services
}

teardown() {
    teardown_new_cluster
    gpupgrade kill-services
    gpstart -a
    echo "done"
}

#
# This test should probably become a journey test for
# more finalize steps that actually go through the process of
# finalizing a cluster (mirrors, etc)
#
@test "finalize brings up the standby for the new cluster" {
    gpupgrade initialize \
        --old-bindir="$GPHOME/bin" \
        --new-bindir="$GPHOME/bin" \
        --old-port="${PGPORT}" \
        --disk-free-ratio 0 \
        --verbose

    gpupgrade execute --verbose

    gpupgrade finalize

    local new_datadir=$(gpupgrade config show --new-datadir)
    local actual_standby_status=$(gpstate -d "${new_datadir}")
    local standby_status_line=$(get_standby_status "$actual_standby_status")
    [[ $standby_status_line == *"Standby host passive"* ]] || fail "expected standby to be up and in passive mode, got **** ${actual_standby_status} ****"
}

get_standby_status() {
    local standby_status=$1
    echo "$standby_status" | grep 'Standby master state'
}

