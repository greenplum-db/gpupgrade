#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load test/helpers

# If GPHOME_NEW is not set, then it defaults to GPHOME, doing a upgrade to the
#  same version

setup() {
    skip_if_no_gpdb

    GPHOME_NEW=${GPHOME_NEW:-$GPHOME}

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services
}

teardown() {
    skip_if_no_gpdb

    if [ -n "$NEW_CLUSTER" ]; then
        delete_finalized_cluster $NEW_CLUSTER
    fi

    gpupgrade kill-services
    rm -r "$STATE_DIR"

    start_source_cluster
}

@test "gpugrade can make it as far as we currently know..." {
    gpupgrade initialize \
              --source-bindir "$GPHOME"/bin \
              --target-bindir "$GPHOME_NEW"/bin \
              --source-master-port $PGPORT \
              --temp-port-range 6020-6040 \
              --disk-free-ratio=0 \
              --verbose \
              3>&-

    gpupgrade execute --verbose
    gpupgrade finalize --verbose

    NEW_CLUSTER="$MASTER_DATA_DIRECTORY"
}
