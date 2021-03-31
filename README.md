# gpupgrade [![Concourse Build Status](https://prod.ci.gpdb.pivotal.io/api/v1/teams/main/pipelines/gpupgrade/badge)](https://prod.ci.gpdb.pivotal.io/teams/main/pipelines/gpupgrade)

gpupgrade runs [pg_upgrade](https://www.postgresql.org/docs/current/static/pgupgrade.html)
across all segments to upgrade a [Greenplum cluster](https://github.com/greenplum-db/gpdb)
across major versions. For further details read the Greenplum Database Upgrade [documentation](https://gpdb.docs.pivotal.io/upgrade/) and [blog post](https://greenplum.org/greenplum-database-upgrade/). 
We warmly welcome any feedback and
[contributions](https://github.com/greenplum-db/gpupgrade/blob/master/CONTRIBUTING.md).

**Purpose:**

Greenplum has several ways of upgrading including backup & restore and gpcopy.
These methods usually require additional diskspace for the required copy and 
significant downtime. gpupgrade can do in-place upgrades without the need 
for additional hardware, disk space, and with less downtime. 

Creating an easy upgrade path enables users to quickly and confidently upgrade. 
This enables Greenplum to have faster release cycles with faster user feedback. 
Most importantly it allows Greenplum to reduce its reliance on supporting legacy 
versions.

**Supported Versions:**

| Source Cluster | Target Cluster
| --- | ---
| 5 | 6
| 6 | 7 (future work)
 
**Architecture:**

gpupgrade consists of three processes that communicate using gRPC and protocol buffers:
- CLI
  - Runs on the master host
  - Consists of a gRPC client
- Hub
  - Runs on the master host
  - Upgrades the master
  - Coordinates the agent processes
  - Consists of a gRPC client and server 
- Agents
  - Run on all segment hosts
  - Upgrade the standby, primary, and mirror segments
  - Execute commands received from the hub
  - Consist of a gRPC server
 
```
       CLI                     Hub                     Agent
      ------                  ------                  -------
  gRPC client    <-->      gRPC server
                                ^
                                |
                                V
                           gRPC client     <-->      gRPC server
```

**Steps:**

Running gpupgrade consists of three main steps:
- gpupgrade initialize
  - The source cluster can still be running. No downtime.
  - Substeps include creating the gpupgrade state directory, starting the hub 
  and agents, creating the target cluster, and running pre-upgrade checks. 
- gpupgrade execute
  - This step will stop the source cluster. Downtime is needed.
  - Substeps include upgrading the master, copying the master catalog to the 
  segments, and upgrading the primaries.  
- gpupgrade finalize
  - After finalizing the upgrade cannot be reverted.
  - Substeps include updating the data directories and master catalog, and 
  upgrading the standby and mirrors.
- gpupgrade revert
  - Restores the cluster to the state before upgrade.
  - Can be run after initialize or execute, but *not* finalize.
  - Substeps include deleting the target cluster, archiving the gpupgrade log 
  directory, and restoring the source cluster.

```
  start <---- run migration
    |            scripts  |
run migration             |
  scripts                 ^
    |                     |
    V                     |
initialize ---> revert ----
    |                     ^
    V                     |
 execute  ----> revert ----
    |
    V
 finalize
    |
run migration 
  scripts
    |
    V
   done
```

Each substep within a step implements [crash-only idempotence](https://en.wikipedia.org/wiki/Crash-only_software).
This means that if an error occurs and is fixed then on rerun the step will 
succeed. This requires each substep to clean up any side effects it creates, 
or possibly check if the work has been done.

**Link vs. Copy Mode:**

pg_upgrade supports two upgrade modes: link and copy.

| Attribute | Copy Mode | Link Mode
| --- | --- | ---
| Description | Copy's source files to the target cluster. | Uses hard links to modify the source cluster data in place.
| Upgrade Time | Slow, since it copy's the data before upgrading. | Fast, since the data is modified in place.
| Disk Space | ~60% free disk space needed. | ~20% free disk space needed.
| Revert Speed | Fast, since the source cluster remains untouched. | Slow, since the source files have been modified the primaries and mirrors need to be rebuilt.
| Risk | Less risky since the source cluster is untouched. | More risky since the source cluster is modified.


## Getting Started

### Prerequisites

- Golang. We currently develop against latest stable Golang, which was v1.16 as of October 2020.
- protoc. This is the compiler for the [gRPC protobuf](https://grpc.io/) 
system which can be installed on macOS with `brew install protobuf`.
- Run `make && make depend-dev` to install other developer dependencies. Note 
make needs to be run first.

### Build and Test

```
make         # builds gpupgrade binary locally
make check   # runs tests
make install # installs gpupgrade into $GOBIN
```

### Running

```
gpupgrade initialize --file ./gpupgrade_config
OR
gpupgrade initialize --source-gphome "$GPHOME" --target-gphome "$GPHOME" --source-master-port 6000 --disk-free-ratio 0
gpupgrade execute
gpupgrade finalize
```

### Running Tests

#### Unit tests
```
make unit
```
#### Integration tests
Tests that run against the gpupgrade binary to verify the interaction between 
components. Before writing a new integration test please review the 
[README](https://github.com/greenplum-db/gpupgrade/blob/master/integrations/README.md).
```
make integration
```
#### Acceptance tests
Tests more end-to-end acceptance-level behavior between components. Tests are 
located in the `test` directory and use the [BATS (Bash Automated Testing System)](https://github.com/bats-core/bats-core) 
framework which can be installed on macOS with `brew install bats-core`.
Please review the [integrations/README](https://github.com/greenplum-db/gpupgrade/blob/master/integrations/README.md).
```
# Some tests require GPDB installed and running
make check-bats
```
#### All local tests
```
# Runs all local tests
make check
```
#### End-to-End tests
Creates a Concourse pipeline that includes various multi-host X-to-Y upgrade and 
functional tests. These cannot be run locally.
```
make set-pipeline
```


## Concourse Pipeline

To update the generated pipeline edit `ci/template.yml` and run 
`make set-pipeline` or `go generate ./ci` which is automatically run as part of
 `make set-pipeline`. This will update `ci/generated/pipeline.yml`.

To update the production pipeline: `PIPELINE_NAME=gpupgrade FLY_TARGET=prod make set-pipeline`

To make the pipeline publicly visible run `make expose-pipeline`. This will 
allow anyone to see the pipeline and its status. However, the task details will 
not be visible unless one logs into Concourse.


## Generating gRPC code

To recompile proto files to generate gRPC client and server code run 
`go generate ./idl`


## Bash Completion

To enable tab completion of gpupgrade commands source the `cli/bash/gpupgrade.bash`
script from your `~/.bash_completion` config, or copy it into your system's 
completions directory such as  `/etc/bash_completion.d`.


## Building

Cross-compile with:
- `make build_linux`
- `make build_mac`
