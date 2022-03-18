# gpupgrade [![Concourse Build Status](https://prod.ci.gpdb.pivotal.io/api/v1/teams/main/pipelines/gpupgrade/badge)](https://prod.ci.gpdb.pivotal.io/teams/main/pipelines/gpupgrade)

gpupgrade runs [pg_upgrade](https://www.postgresql.org/docs/current/static/pgupgrade.html)
across all segments to upgrade a [Greenplum cluster](https://github.com/greenplum-db/gpdb)
across major versions. For further details read the Greenplum Database Upgrade [documentation](https://gpdb.docs.pivotal.io/upgrade/) and [blog post](https://greenplum.org/greenplum-database-upgrade/). 
We warmly welcome any feedback and
[contributions](https://github.com/greenplum-db/gpupgrade/blob/master/CONTRIBUTING.md).

**Purpose:**

Greenplum has multiple ways of upgrading including backup & restore and gpcopy.
These methods usually require additional diskspace for the required copy and 
significant downtime. gpupgrade can do fast in-place upgrades without the need 
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

Running gpupgrade consists of several steps (ie: commands):
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

Optional steps (ie: commands):
- gpupgrade revert
  - To restore the cluster to the state before the upgrade.
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

gpupgrade inits a fresh target cluster "next to" the source cluster, and upgrades 
"into it" in-place using pg_upgrade's copy or link mode.

| Attribute | Copy Mode | Link Mode
| --- | --- | ---
| Description | Copy's source files to the target cluster. | Uses hard links to modify the source cluster data in place.
| Upgrade Time | Slow, since it copy's the data before upgrading. | Fast, since the data is modified in place.
| Disk Space | ~60% free disk space needed. | ~20% free disk space needed.
| Revert Speed | Fast, since the source cluster remains untouched. | Slow, since the source files have been modified the primaries and mirrors need to be rebuilt.
| Risk | Less risky since the source cluster is untouched. | More risky since the source cluster is modified.


## Getting Started

### Prerequisites

- Golang. See the top of [go.mod](go.mod) for the current version used.
- protoc. This is the compiler for the [gRPC protobuf](https://grpc.io/) 
system which can be installed on macOS with `brew install protobuf`.
- Run `make && make depend-dev` to install other developer dependencies. Note 
make needs to be run first.

### Setting up your IDE

#### Vim

Checkout [vim-go](https://github.com/fatih/vim-go) and [go-delve](https://github.com/go-delve/delve).

#### IntelliJ

##### Imports
- Preferences > Editor > Code Style > Go > select "Imports" tab
  - uncheck "Use back quotes for imports"
  - uncheck "Add parentheses for a single import"
  - uncheck "Remove redundant import aliases"
  - Sorting type: gofmt
  - check "Move all imports in a single declaration"
  - check "Group stdlib imports"
    - check "Move all stdlib imports in a single group"
  - check "Group"
    - check "Current project packages"

##### Copyright
- Preferences > Editor > Copyright > Copyright Profiles
  - Add new profile called "vmware" with the following text:
    
    ```
    // Copyright (c) 2017-2021 VMware, Inc. or its affiliates
    // SPDX-License-Identifier: Apache-2.0
    ```
- Preferences > Editor > Copyright > Formatting > Go
  - select "Use custom formatting options"
  - For Comment Type: check "Prefix each line" and select "use line comment"
  - For Relative Location: select "Before other comments" and check "Add blank line after"

##### Formatting
- Install the "File Watchers" plugin
- Preferences > Tools > File Watchers
  - Add a new file watcher choosing "go fmt"
  - Depending on your personal preference check "Auto-save edited files to trigger the watcher"
- Now saving the file (ctl+s) will format the file.
- References: https://blog.jetbrains.com/go/2021/01/12/running-go-fmt-on-save/

### Build and Test

```
make         # builds gpupgrade binary locally
make check   # runs tests
make install # installs gpupgrade into $GOBIN
```

Cross-compile with:
- `make build_linux`
- `make build_mac`

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
make gpupgrade-tests
make pg-upgrade-tests
```

To run all tests in a suite:
```
bats test/acceptance/gpupgrade/finalize.bats
```

To run a single test or set of tests:
```
bats -f "gpupgrade finalize should" test/acceptance/gpupgrade/finalize.bats
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

To update the pipeline edit the yaml files in the `ci` directory and run 
`make set-pipeline`. 

The yaml files in the `ci` directory are concatenated to 
create `ci/generated/template.yml`. Next, `go generate ./ci` is executed which 
runs `go run ./parser/parse_template.go generated/template.yml generated/pipeline.yml`
to create `ci/generated/pipeline.yml`. None of the generated files `template.yml` 
or `pipeline.yml` are checked in.

To update the production pipeline: `PIPELINE_NAME=gpupgrade FLY_TARGET=prod make set-pipeline`

To make the pipeline publicly visible run `make expose-pipeline`. This will 
allow anyone to see the pipeline and its status. However, the task details will 
not be visible unless one logs into Concourse.

*Note:* If your dev pipeline is failing on the build job while verifying the rpm 
then the most likely cause is needing to sync the latest tags on origin with 
your remote. This allows the GPDB test rpm to have the correct version number. 
On your GPDB branch run the following:
```
$ git fetch --tags origin
$ git push --tags <yourRemoteName>
```
If you already flew a pipeline *before* pushing tags you will likely 
need to delete it, push tags, and re-fly as Concourse has some weird caching 
issues.


## Generating gRPC code

To recompile proto files to generate gRPC client and server code run 
`go generate ./idl`


## Bash Completion

To enable tab completion of gpupgrade commands source the `cli/bash/gpupgrade.bash`
script from your `~/.bash_completion` config, or copy it into your system's 
completions directory such as  `/etc/bash_completion.d`.

## Debugging
- Identify the High Level Failure
  - What mode was used - copy vs. link?
  - What step failed - initialize, execute, finalize, or revert?
  - What specific substep failed?
- Identify the Failing Host
  - Did the Hub (master) vs. Agent (segment) fail?
  - What specific host failed?
- Identify the Failed Utility
  - Did gpupgrade fail, or an underlying utility such as pg_upgrade, gpinitsystem, gpstart, etc.?
- Identify the Specific Failure
  - Based on the error context and logs what is the specific error?

### Debugging Hub and Agent Processes
- Set a breakpoint in the CLI
  - For example in `cli/commands/initialize.go`, `execute.go`, or `finalize.go` right before the call to the hub.
- Set additional breakpoints in the hub or agent code to aid in debugging.
- Run gpupgrade to hit the first breakpoint in the CLI process.
- When using intellij "Attach to Process" and select the hub and agent processes.
- Continue execution until the additional breakpoints in the hub or agent code are hit. Step through the code to debug.
- For faster iterations:
  - Make any local changes in the code
  - Rebuild with `make && make install`
  - Reload the new code with `gpupgrade restart-services` or manually stop and restart the hub.
  - Repeat the above breakpoints and attaching to the new processes as their PIDs have changed.
