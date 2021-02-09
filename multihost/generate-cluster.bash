#!/usr/bin/env bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

GPUPGRADE_SOURCE_PATH=/vagrant

STANDBY_OPTS="-s standby-agent.local"
GP_INIT_SYSTEM="cd $GPUPGRADE_SOURCE_PATH/multihost && gpinitsystem -a -c gpupgrade_cluster_config ${STANDBY_OPTS}"

vagrant ssh hub --command="$GP_INIT_SYSTEM"
