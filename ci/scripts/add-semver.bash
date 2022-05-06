#!/bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# Rename the pipeline-local rpms to be unique per git SHA.
# gpupgrade-intermediate.el7.x86_64.rpm -> gpupgrade-0.4.0+dev.32.g763a08e5-1.el7.x86_64.rpm

function rename_rpm() {
    local release=$1

    rpm=$(basename "${release}_rpm"/gpupgrade-*.rpm)
    # shellcheck disable=2001
    # NOTE: The "-1" is the packing build number which enables us to release
    # a new package when there are no code changes.
    sed s/gpupgrade-intermediate/gpupgrade-"${SEMVER}-1"/ <<< "$rpm"
}

# "git describe" does not return a semver compatible version that can correctly be used with Concourse to
# ensure pulling the correct artifact. So "0.4.0-32-g763a08e5" becomes "0.4.0+dev.32.g973669ba".
git_describe=$(git -C ./gpupgrade_src describe --tags)
IFS='-' read -r -a parts <<< "$git_describe"
if (( ${#parts[@]} == 1 )); then
    SEMVER="${parts[0]}"
    cp oss_rpm/gpupgrade-*.rpm renamed_oss_rpm/"$(rename_rpm oss)"
    cp enterprise_rpm/gpupgrade-*.rpm renamed_enterprise_rpm/"$(rename_rpm enterprise)"
elif (( ${#parts[@]} == 3 )); then
    SEMVER="${parts[0]}+dev.${parts[1]}.${parts[2]}"
    cp oss_rpm/gpupgrade-*.rpm renamed_oss_rpm/"$(rename_rpm oss)"
    cp enterprise_rpm/gpupgrade-*.rpm renamed_enterprise_rpm/"$(rename_rpm enterprise)"
else
    echo "git describe '${git_describe}' was split into ${#parts[@]} parts [${parts[*]}]. Expected 1 or 3."
    exit 1
fi
