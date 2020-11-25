#! /usr/bin/env bats
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

# TODO: test also that we have a git version in the version string as well

@test "gpupgrade version prints version" {
    run gpupgrade version
    check_version
}

@test "gpupgrade --version prints version" {
    run gpupgrade --version
    check_version
}

@test "gpupgrade -V prints version" {
    run gpupgrade -V
    check_version
}

check_version() {
    [ "$status" -eq 0 ]
    [[ "${lines[0]}" =~ "Version:"[[:space:]][[:digit:]]\.[[:digit:]]\.[[:digit:]] ]]
    [[ "${lines[1]}" =~ "Commit:"[[:space:]][[:alnum:]] ]]
    [[ "${lines[2]}" = "Release: Dev Build" ]]
}
