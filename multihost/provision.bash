#!/usr/bin/env bash
#
# Copyright (c) 2017-2021 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

GPUPGRADE_SOURCE_PATH=/vagrant
VAGRANT_USER_HOME=/home/vagrant

own_directories() {
    sudo chown -R vagrant /usr/local
}

# Install dependencies
modify_linux_configuration_for_greenplum() {
    #
    # see README.Centos.md in gpdb repository
    #

    # modify sysctl
    sudo bash -c 'cat >> /etc/sysctl.conf <<-EOF
kernel.shmmax = 500000000
kernel.shmmni = 4096
kernel.shmall = 4000000000
kernel.sem = 500 1024000 200 4096
kernel.sysrq = 1
kernel.core_uses_pid = 1
kernel.msgmnb = 65536
kernel.msgmax = 65536
kernel.msgmni = 2048
net.ipv4.tcp_syncookies = 1
net.ipv4.ip_forward = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.tcp_tw_recycle = 1
net.ipv4.tcp_max_syn_backlog = 4096
net.ipv4.conf.all.arp_filter = 1
net.ipv4.ip_local_port_range = 1025 65535
net.core.netdev_max_backlog = 10000
net.core.rmem_max = 2097152
net.core.wmem_max = 2097152
vm.overcommit_memory = 2
EOF'

    # modify limits to allow more open files
    sudo bash -c 'cat >> /etc/security/limits.conf <<-EOF
* soft nofile 65536
* hard nofile 65536
* soft nproc 131072
* hard nproc 131072

EOF'

    # reload sysctl configuration
    sudo sysctl -p
}


install_yum_packages() {
    # Add Inline with Upstream Stable (IUS) yum repository
    sudo yum install --assumeyes \
        https://repo.ius.io/ius-release-el7.rpm \
        https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm

    # Add Extra Packages for Enterprise Linux (EPEL)
    sudo yum install epel-release --assumeyes

    sudo yum install --assumeyes \
        git2u \
        go \
        avahi \
        nss-mdns \
        emacs-nox \
        net-tools \
        wget
}

install_bats() {
    #
    # Download from Github because centos only
    # has v0.4.0 available via yum
    #
    wget https://github.com/bats-core/bats-core/archive/v1.1.0.tar.gz
    tar xvzf v1.1.0.tar.gz
    rm v1.1.0.tar.gz
    ./bats-core-1.1.0/install.sh /usr/local
}

install_greenplum() {
    # assumes that the gpdb rpm has been manually downloaded
    # and has been placed in the multihost directory
    pushd "$GPUPGRADE_SOURCE_PATH/multihost";
        sudo yum install greenplum-db-*.rpm --assumeyes
    popd
    echo ". /usr/local/greenplum-db/greenplum_path.sh" >> $VAGRANT_USER_HOME/.bashrc

    modify_linux_configuration_for_greenplum
}

generate_ssh_keys() {
    ssh-keygen -f /home/vagrant/.ssh/id_rsa -t rsa -P ""
}

install_gpupgrade() {
    cd "$GPUPGRADE_SOURCE_PATH" || exit

    echo 'export PATH=$PATH:/home/vagrant/go/bin' >> $VAGRANT_USER_HOME/.bashrc

    source "$VAGRANT_USER_HOME/.bashrc"

    make install
}

# TODO: Instead of hardcoding hostnames to an ip, configure DNS to be able to resolve
# the hostname
add_hosts_entries() {
    sudo echo "192.168.100.2 standby-agent.local" >> /etc/hosts
    sudo echo "192.168.100.3 segment-agent.local" >> /etc/hosts
    sudo echo "192.168.100.4 hub.local" >> /etc/hosts
}

install_dependencies() {
    install_yum_packages
    install_bats
    install_greenplum
    install_gpupgrade
}

setup_dns() {
    # the avahi service is not started by default
    sudo systemctl start avahi-daemon
}

main() {
    own_directories
    install_dependencies
    generate_ssh_keys
    setup_dns
    add_hosts_entries
}

main
