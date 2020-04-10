#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0
#
# Set up a multi-host Vagrant environment
# for gpupgrade.
#
# see README.md for more information
#

class Host
    def initialize(name:, memory:)
        @name = name
        @memory = memory
    end

    def define(config, ip_address)
        config.vm.define name do |guest|
            guest.vm.network "private_network", ip: ip_address

            guest.vm.hostname = "#{name}.local"
            guest.vm.provision "shell",
                path: "multihost/provision-#{name}.bash",
                privileged: false

            # virtualbox specific overrides
            guest.vm.provider "virtualbox" do |vb|
              vb.memory = memory
            end
        end
    end

    private

    attr_reader :name, :memory
end

def configure_multihost(config)
    config.vm.box = "centos/7"

    # provisioning applicable to all hosts
    config.vm.provision "shell",
        path: "multihost/provision.bash",
        privileged: true

    # enable two-way shared folders via nfs
    config.vm.synced_folder ".", "/vagrant", type: "nfs"

    hosts = [
        Host.new(name: "standby-agent", memory: "2048"),
        Host.new(name: "segment-agent", memory: "8192"),
        Host.new(name: "hub", memory: "2048"),
    ]

    hosts.each_with_index do |host, index|
        host.define(config, "192.168.100.#{index+2}")
    end
end
