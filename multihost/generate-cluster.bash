#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

GPUPGRADE_SOURCE_PATH=/vagrant

COMMAND="cd $GPUPGRADE_SOURCE_PATH/multihost && gpinitsystem -a -c gpupgrade_cluster_config"

vagrant ssh hub --command="$COMMAND"
