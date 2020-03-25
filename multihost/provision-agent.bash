#!/usr/bin/env bash

main() {
    # make cluster directories, required by gpinitsystem
    mkdir -p /home/vagrant/gpdb-cluster/primary
    mkdir -p /home/vagrant/gpdb-cluster/mirror
}

main
