# gpupgrade multi-host testing

Multi-host environment for gpupgrade

Creates a two hosts via vagrant. One that runs the hub,
one that runs the agent, and a gpdb cluster with a master on the
hub host, and three segments and three mirrors on the agent host.

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
