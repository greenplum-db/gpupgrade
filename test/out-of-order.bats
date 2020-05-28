#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# This file provides negative test cases for when the user does not execute
# upgrade steps in the correct order after starting the hub.

load helpers

setup() {
    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services

    gpupgrade initialize \
        --source-bindir="${GPHOME_SOURCE}/bin" \
        --target-bindir="${GPHOME_TARGET}/bin" \
        --source-master-port="${PGPORT}" \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-
}

teardown() {
    gpupgrade kill-services
    rm -r "${STATE_DIR}"
}

# todo: add tests
