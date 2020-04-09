#
# Set up a multi-host Vagrant environment
# for gpupgrade.
#
# see README.md for more information
#
def configure_multihost(config)
    config.vm.box = "centos/7"

    # provisioning applicable to all hosts
    config.vm.provision "shell",
        path: "multihost/provision.bash",
        privileged: false

    # enable two-way shared folders via nfs
    config.vm.synced_folder ".", "/vagrant", type: "nfs"

    # gpupgrade hub host
    config.vm.define "hub" do |guest|
        guest.vm.network "private_network", ip: "192.168.100.2"
        guest.vm.hostname = "hub.local"
        guest.vm.provision "shell",
            path: "multihost/provision-hub.bash",
            privileged: false

        # virtualbox specific overrides
        guest.vm.provider "virtualbox" do |vb|
          vb.memory = "2048"
        end
    end

    # gpupgrade agent host
    config.vm.define "agent" do |guest|
        guest.vm.network "private_network", ip: "192.168.100.3"
        guest.vm.hostname = "agent.local"
        guest.vm.provision "shell",
            path: "multihost/provision-agent.bash",
            privileged: false

        # virtualbox specific overrides
        guest.vm.provider "virtualbox" do |vb|
          vb.memory = "8192"
        end
    end
end