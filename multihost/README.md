# gpupgrade multi-host testing

Multi-host environment for gpupgrade

Creates three hosts via vagrant. One that runs the hub,
one that runs the agent for the segments, and one that runs the
agent for the standby coordinator.

Also, a gpdb cluster with a coordinator on the `hub` host, three
segments on the `segment-agent` host, three mirrors on the
`segment-agent` host, and one standby coordinator on the
`standby-agent` host.

## Useful links for configuring Vagrant

- https://docs.vagrantup.com
- https://vagrantcloud.com/search
- https://www.vagrantup.com/docs/multi-machine/

## Setup:

* The latest GPDB6 RPM can be found on Tanzu Network. Download the Centos 7 rpm of the Greenplum Database Server and place it in the `gpupgrade/multihost` directory

    https://network.pivotal.io/products/pivotal-gpdb

* Create the hub and agent hosts

    ```bash
    # requires host sudo password for configuring NFS
    vagrant up
    ```

* Setup passwordless ssh between hosts

    ```bash
    ./multihost/sync-ssh-keys.bash
    ```

* Generate a greenplum cluster

    ```bash
    ./multihost/generate-cluster.bash
    ```
