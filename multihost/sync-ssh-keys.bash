#!/usr/bin/env bash
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

# Vagrant guest names
hostnames=(hub standby-agent segment-agent)

# guest files
public_key_path=/home/vagrant/.ssh/id_rsa.pub
guest_authorized_keys_path=/home/vagrant/.ssh/authorized_keys
guest_known_hosts_path=/home/vagrant/.ssh/known_hosts

# host files
authorized_keys_path="./.vagrant/host_authorized_keys"
known_hosts_path="./.vagrant/host_known_hosts"

# empty authorized keys file
echo -n "" > $authorized_keys_path
echo -n "" > $known_hosts_path

for hostname in "${hostnames[@]}"; do
    echo
    echo "Collecting ssh authorized keys for ${hostname}..."
    vagrant ssh "$hostname" --command="cat $public_key_path" >> $authorized_keys_path
    vagrant ssh "$hostname" --command="cat $guest_authorized_keys_path" >> $authorized_keys_path
    echo "Done."
done

for hostname in "${hostnames[@]}"; do
    echo
    echo "Collecting ssh known hosts for ${hostname}..."
    for inner_hostname in "${hostnames[@]}"; do
        vagrant ssh "$hostname" --command="ssh-keyscan $inner_hostname.local >> $guest_known_hosts_path"
    done

    vagrant ssh "$hostname" --command="cat $guest_known_hosts_path" >> "$known_hosts_path"
    echo "Done."
done

for hostname in "${hostnames[@]}"; do
    echo
    echo "Uploading ssh config files for ${hostname}..."
    vagrant upload $authorized_keys_path $guest_authorized_keys_path "$hostname"
    vagrant upload $known_hosts_path $guest_known_hosts_path "$hostname"
    vagrant ssh $hostname --command="sudo chown vagrant:vagrant -R /home/vagrant"
    echo "Done."
done

# Cleanup
rm $authorized_keys_path
rm $known_hosts_path
