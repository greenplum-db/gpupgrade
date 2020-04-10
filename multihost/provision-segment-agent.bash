#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

main() {
    # make cluster directories, required by gpinitsystem
    mkdir -p /home/vagrant/gpdb-cluster/primary
    mkdir -p /home/vagrant/gpdb-cluster/mirror
}

main
