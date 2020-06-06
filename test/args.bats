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

@test "gpupgrade subcommands fail when passed insufficient arguments" {
    run gpupgrade initialize
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'Required flag(s) "source-bindir", "source-master-port", "target-bindir" have/has not been set'* ]]; then
        fail "actual: $output"
    fi

    run gpupgrade config set
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'the set command requires at least one flag to be specified'* ]]; then
        fail "actual: $output"
    fi
}

@test "gpupgrade initialize fails when other flags are used with --file" {
    run gpupgrade initialize --file /some/config --source-bindir /old/bindir
    [ "$status" -eq 1 ]
    if ! [[ "$output" = *'The file flag cannot be used with any other flag'* ]]; then
        fail "actual: $output"
    fi
}

@test "gpupgrade initialize --file with verbose uses the configured values" {
    config_file=${STATE_DIR}/gpupgrade_config
    cat <<- EOF > "$config_file"
		source-bindir = /my/old/bin/dir
		target-bindir = /my/new/bin/dir
		source-master-port = ${PGPORT}
		disk-free-ratio = 0
		stop-before-cluster-creation = true
	EOF

    gpupgrade initialize --verbose --file "$config_file"

    run gpupgrade config show --target-bindir
    [ "$status" -eq 0 ]
    [ "$output" = "/my/new/bin/dir" ]

    run gpupgrade config show --source-bindir
    [ "$status" -eq 0 ]
    [ "$output" = "/my/old/bin/dir" ]
}
