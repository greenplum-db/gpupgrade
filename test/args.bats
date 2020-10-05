#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    rm -r "$STATE_DIR"
}

@test "gpupgrade initialize fails when passed insufficient arguments" {
    run gpupgrade initialize
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'required flag(s) "source-gphome", "source-master-port", "target-gphome" not set'* ]]; then
        fail "actual: $output"
    fi
}

@test "gpupgrade initialize fails when other flags are used with --file" {
    run gpupgrade initialize --file /some/config --source-gphome /usr/local/source
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'The file flag cannot be used with any other flag'* ]]; then
        fail "actual: $output"
    fi
}

@test "gpupgrade initialize --file with verbose uses the configured values" {
    config_file=${STATE_DIR}/gpupgrade_config
    cat <<- EOF > "$config_file"
		source-gphome = $GPHOME_SOURCE
		target-gphome = $GPHOME_TARGET
		source-master-port = ${PGPORT}
		disk-free-ratio = 0
		stop-before-cluster-creation = true
	EOF

    gpupgrade initialize --verbose --file "$config_file"

    run gpupgrade config show --source-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "$GPHOME_SOURCE" ]

    run gpupgrade config show --target-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "$GPHOME_TARGET" ]
}

@test "initialize sanitizes source-gphome and target-gphome" {
    gpupgrade initialize \
        --source-gphome "${GPHOME_SOURCE}/" \
        --target-gphome "${GPHOME_TARGET}//" \
        --source-master-port ${PGPORT} \
        --stop-before-cluster-creation \
        --disk-free-ratio 0 3>&-

    run gpupgrade config show --source-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "$GPHOME_SOURCE" ]

    run gpupgrade config show --target-gphome
    [ "$status" -eq 0 ]
    [ "$output" = "$GPHOME_TARGET" ]
}
