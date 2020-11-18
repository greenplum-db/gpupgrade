#!/bin/bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux -o pipefail

RPM=$1
RELEASE=$2
VERSION=$(git describe --tags --abbrev=0)

verify_gpugprade_version_output() {
  [[ $(/usr/local/greenplum-upgrade/gpupgrade version) == *"Version: ${VERSION}"* ]]
  [[ $(/usr/local/greenplum-upgrade/gpupgrade version) == *"Release: ${RELEASE}"* ]]
}

verify_rpm_info() {
  local info="$1"

  [[ $info == *"Name        : greenplum-upgrade"* ]]
  [[ $info == *"Architecture: x86_64"* ]]
  [[ $info == *"Source RPM  : greenplum-upgrade-${VERSION}-1"* ]]
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
  if [ "$RELEASE" = "Open Source" ]; then
      [ ! -s "/usr/share/licenses/greenplum-upgrade*/open_source_licenses.txt" ]
      [ ! -s "/usr/local/greenplum-upgrade/open_source_licenses.txt" ]
      return
  fi

  # For ENTERPRISE release, sanity check the license file
  [ -s "/usr/share/licenses/greenplum-upgrade-${VERSION}/open_source_licenses.txt" ]

  local license_file="/usr/local/greenplum-upgrade/open_source_licenses.txt"
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
  verify_rpm_info "$(rpm -qi greenplum-upgrade)"
  verify_license_files

  rpm -ev greenplum-upgrade
}

main
