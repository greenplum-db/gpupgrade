#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

main() {
    # make cluster directory, required by gpinitsystem
    mkdir -p /home/vagrant/gpdb-cluster/qddir

    # setup gpdb utilities enviroment
    echo "export PGPORT=6000" >> $HOME/.bashrc
    echo "export MASTER_DATA_DIRECTORY=/home/vagrant/gpdb-cluster/qddir/demoDataDir-1" >> $HOME/.bashrc
}

main
