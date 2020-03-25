#!/usr/bin/env bash

GPUPGRADE_SOURCE_PATH=/vagrant

COMMAND="cd $GPUPGRADE_SOURCE_PATH/multihost && gpinitsystem -a -c gpupgrade_cluster_config"

vagrant ssh hub --command="$COMMAND"
