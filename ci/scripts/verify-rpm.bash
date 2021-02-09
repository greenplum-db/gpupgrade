#!/bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

RPM=$1
RELEASE=$2
VERSION=$(git describe --tags --abbrev=0)

verify_gpugprade_version_output() {
  [[ $(/usr/local/bin/gpupgrade version) == *"Version: ${VERSION}"* ]]
  [[ $(/usr/local/bin/gpupgrade version) == *"Release: ${RELEASE}"* ]]
}

verify_rpm_info() {
  local info="$1"

  [[ $info == *"Name        : gpupgrade"* ]]
  [[ $info == *"Architecture: x86_64"* ]]
  [[ $info == *"Source RPM  : gpupgrade-${VERSION}-1"* ]]
  [[ $info == *"URL         : https://github.com/greenplum-db/gpupgrade"* ]]

  if [ "$RELEASE" = "Open Source" ]; then
      [[ $info == *"License     : Apache 2.0"* ]]
      [[ $info == *"Summary     : Greenplum Database Upgrade"* ]]
      return
  fi

  [[ $info == *"License     : VMware Software EULA"* ]]
  [[ $info == *"Summary     : VMware Tanzu Greenplum Upgrade"* ]]
}

verify_license_files() {
  local license_file="/usr/local/bin/greenplum/gpupgrade/open_source_licenses.txt"
  [ -s "$license_file" ]

  [[ $(head -1 "$license_file") =~ open_source_licenses.txt ]]
  [[ $(head -3 "$license_file" | tail -1) == *"VMware Tanzu Greenplum Upgrade ${VERSION}"* ]]
  [[ $(tail -1 "$license_file") =~ "GREENPLUMUPGRADE" ]]
}

main() {
  [ -f "$RPM" ]
  [ "$RELEASE" = "Enterprise" ] || [ "$RELEASE" = "Open Source" ]

  rpm -ivh "$RPM"
  verify_gpugprade_version_output
  verify_rpm_info "$(rpm -qi gpupgrade)"
  verify_license_files

  rpm -ev gpupgrade
}

main
