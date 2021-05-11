#! /bin/bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -eux

cp -R cluster_env_files/.ssh /root/.ssh

# Load the finalize test library.
source gpupgrade_src/test/acceptance/helpers/finalize_checks.bash

echo 'Doing failover tests of mirrors and standby...'
validate_mirrors_and_standby /usr/local/greenplum-db-target mdw 5432
