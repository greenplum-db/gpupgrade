# gpupgrade [![Concourse Build Status](https://prod.ci.gpdb.pivotal.io/api/v1/teams/main/pipelines/gpupgrade/badge)](https://prod.ci.gpdb.pivotal.io/teams/main/pipelines/gpupgrade)

gpupgrade runs [pg_upgrade](https://www.postgresql.org/docs/current/static/pgupgrade.html)
across all segments to quickly upgrade a [Greenplum cluster](https://github.com/greenplum-db/gpdb)
across major versions. Since it's still being actively developed it should not 
be used in production at this time. We warmly welcome any feedback and 
[contributions](https://github.com/greenplum-db/gpupgrade/blob/master/CONTRIBUTING.md).

**Architecture:**

gpupgrade consists of three processes that communicate using gRPC and protocol buffers:
- CLI
  - Runs on the master host
  - Consists of a gRPC client
- Hub
  - Runs on the master host
  - Coordinates the agent processes
  - Consists of a gRPC client and server 
- Agents
  - Run on all segment hosts
  - Execute commands received from the hub
  - Consist of a gRPC server
 
```
       CLI                     Hub                     Agent
      ------                  ------                  -------
  gRPC client    <-->      gRPC server
                           gRPC client     <-->      gRPC server
```

**Steps:**

Running gpupgrade consists of three main steps:
- gpupgrade initialize
  - Then source cluster can still be running. No downtime.
  - Substeps include creating the gpupgrade state directory, starting the hub 
  and agents, creating the target cluster, and running pg_ugpgrade checks. 
- gpupgrade execute
  - This step will stop the source cluster. Downtime is needed.
  - Substeps include upgrading the master, copying the master catalog to the 
  segments, and upgrading the primaries.  
- gpupgrade finalize
  - After finalizing the upgrade cannot be reverted.
  - Substeps include updating the data directories and master catalog, and 
  upgrading the standby and mirrors.


## Getting Started

### Prerequisites

- Golang. We currently develop against latest stable Golang, which was v1.14 as of April 2020.
- protoc. This is the compiler for the [gRPC protobuffer](https://grpc.io/) system.
On Mac use `brew install protobuf`.
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
gpupgrade initialize --source-bindir "$GPHOME/bin" --target-bindir "$GPHOME/bin" --source-master-port 6000 --disk-free-ratio 0
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
located in the `test` directory and use BATS (Bash Automated Testing System) framework.
Please review the [integrations/README](https://github.com/greenplum-db/gpupgrade/blob/master/integrations/README.md).
```
# Some tests require GPDB installed and running
make check-bats
```
#### Installcheck tests

Runs through an upgrade on the locally running GPDB cluster.
```
# Requires GPDB installed and running
make installcheck
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

To make the pipeline publicly visible run `make expose-pipeline`. This will 
allow anyone to see the pipeline and its status. However, the task details will 
not be visible unless one logs into Concourse.


## Generating gRPC code

To recompile proto files to generate gRPC client and server code run 
`go generate ./idl`


## Bash Completion

To enable tab completion of gpupgrade commands source the `cli/bash/bash-completion.sh` 
script from your `~/.bash_completion` config, or copy it into your system's 
completions directory such as  `/etc/bash_completion.d`.


## Building

Cross-compile with:
- `make build_linux`
- `make build_mac`
