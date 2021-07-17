#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

# Rename the pipeline-local rpms to be unique per git SHA.
# gpupgrade-intermediate.el7.x86_64.rpm -> gpupgrade-0.4.0+dev.32.g763a08e5-1.el7.x86_64.rpm

function rename_rpm() {
    local release=$1

    rpm=$(basename "rpm_${release}"/gpupgrade-*.rpm)
    # shellcheck disable=2001
    sed s/gpupgrade-intermediate/gpupgrade-"${SEMVER}-1"/ <<< "$rpm"
}

# "git describe" does not return a semver compatible version that can correctly be used with Concourse to
# ensure pulling the correct artifact. So "0.4.0-32-g763a08e5" becomes "0.4.0+dev.32.g973669ba".
IFS='- ' read -r -a parts <<< "$(git -C ./gpupgrade_src describe --tags)"
SEMVER="${parts[0]}+dev"
if [ -n "${parts[1]}" ]; then
  SEMVER="${SEMVER}.${parts[1]}.${parts[2]}"
fi

cp rpm_oss/gpupgrade-*.rpm renamed_rpm_oss/"$(rename_rpm oss)"
cp rpm_enterprise/gpupgrade-*.rpm renamed_rpm_enterprise/"$(rename_rpm enterprise)"
