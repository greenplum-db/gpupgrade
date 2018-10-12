# gpupgrade [![Concourse Build Status](https://prod.ci.gpdb.pivotal.io/api/v1/teams/main/pipelines/gpupgrade/badge)](https://prod.ci.gpdb.pivotal.io/teams/main/pipelines/gpupgrade)

`gpupgrade` is a Go utility for coordinating
[pg_upgrade](https://www.postgresql.org/docs/current/static/pgupgrade.html)
across all segments in a Greenplum cluster. It's in heavy development, and
should not be used for production environments at this time -- but if you'd like
to help us hack on and test gpupgrade in a development environment, we'd welcome
any feedback you have!

## Developer Workflow

### Prerequisites

- Golang. We currently develop against latest stable Golang, which was v1.10 as of May 2018.
- protoc. This is the compiler for the [gRPC protobuffer](https://grpc.io/) system.
On macOS, one way to install this is via `brew install protobuf`

If you want to hack on the code, please run the `make depend-dev` target, as
this will install test and developer dependencies.

### Build and test the upgrade tool

```
make depend  # run this before the first build; it installs required Go utilities

make
make check   # runs tests
make install # requires a running GPDB instance, for now
```

### Dependency vendoring

gpupgrade uses go dep to vendor its dependencies.

To view the state of dependencies in the project, use the command
```
dep status
```

To add add new dependencies, use the command
```
dep ensure -add [package import string]
```

To update existing dependencies, use the command
```
dep ensure -update
```

For additional information please refer to the [dep documentation](https://golang.github.io/dep/docs/daily-dep.html)

### Build details

```
make
```
builds the code without running the tests.

We build with a ldflag to set the version of the code based on the current
git SHA and latest tag at build time.

We build into $GOPATH/bin/gpupgrade and expect that, after a build,
`which gpupgrade` points to the binary that you just built, assuming
your PATH is configured correctly.

We build as part of our (integration tests)[#integration-testing]; see more
information there.

We support cross-compilation into Linux or Darwin, as the GPDB servers that
this tool upgrades run Linux but many dev workstations run MacOS

For a target-specific build, run
```
make build_linux
```
or
```
make build_mac
```
as appropriate for the target platform.

### Run the tests

We use [ginkgo](https://github.com/onsi/ginkgo) and [gomega](https://github.com/onsi/gomega) to run our tests. We have `unit` and `integration` targets predefined.

***Note:*** In order to run integration tests you need to have GPDB installed and running. Instructions to set up a GPDB cluster can be found in [the main GPDB repo](https://github.com/greenplum-db/gpdb).

#### Unit tests
```
# To run only the unit tests
make unit
```
#### Integration tests
```
# To run only the integration tests
make integration
```
#### All tests
```
# To run all the tests
make check
```

### Generate mocked gRPC client/server code
```
# To generate mocked files
make protobuf
```

## Command line parsing

We are using [the cobra library](https://github.com/spf13/cobra) for
parsing our commands and flags.

To implement a new command, you will need to implement a commander (see the cobra documentation and files in the `cli/commanders` directory for examples) and add it to the tree of commands in `cli/gpupgrade_main.go` to tell the parser about your new command.

### Bash Completion

You can source the `cli/bash/bash-completion.sh` script from your
`~/.bash_completion` config, or copy it into your system's completions directory
(e.g.  `/etc/bash_completion.d` on Debian), to get tab completion for the
gpupgrade utility.

## Testing

### Unit testing overall

```
make unit
```
should only run the unit tests

We use Ginkgo and Gomega because they provide BDD-style syntax while still
running `go test` under the hood. Core team members strive to TDD the code as
much as possible so that the unit test coverage is driven out alongside the code

We use dependency injection wherever possible to enable isolated unit tests
and drive towards clear interfaces across packages

Some unit tests depend on the environment variable GPHOME, though not on its value. Having GPHOME set is a prerequisite for running a GPDB cluster.

### Integration testing

```
make integration
```
should only run the integration tests

In order to run the integration tests, a Greenplum Database cluster must be up and
running.
We typically integration test the "happy path" (expected behavior) of the code
when writing new features and allow the unit tests to cover error messaging
and other edge cases. We are not strict about outside-in (integration-first)
or inside-out (unit-first) TDD.

The default integration tests do not build with the special build flags that
the Makefile uses because the capability of the code to react to those build
flags is specifically tested where needed, for example in
[version_integration_test.go](integrations/version_integration_test.go)

The integration tests may require other binaries to be built. We aim to have
any such requirements automated.

### Usage of the Command Line Interface(CLI)

While not "testing" per se, we also describe the gpupgrade process from a database 
maintainer's standpoint.  Since this is the cli, we describe this in a separate
document, ```README_cli.md```  Note that, once officially released, this user
documentation will probably be incorporated into a more user-friendly format.

### Directly using pg_upgrade

Under the covers, gpupgrade is calling pg_upgrade, first on the master, and
then on the segments. If needed, you can call pg_upgrade directly. There is
make target that runs a test, upgrading from version x to x. To do this, two
clusters are setup on the local machine using demo_cluster.sh. In the root
directory for the gpdb repo, run is `make -C contrib/pg_upgrade check`. This
uses test_gpdb.sh to do the heavy lifting, and that can be customized to fit
your setup. In particular, four env vars are used for the cluster mapping:
NEWBINDIR, OLDBINDIR, NEWDATADIR and OLDDATADIR.

### Running tests in a pipeline

The gpupgrade/ci directory contains a pipeline.yml file, which references task
files in gpupgrade/ci/tasks, and some secrets in a private repository. To set a
pipeline, run:

```
fly -t [target-name] set-pipeline -p [pipeline-name] -c gpupgrade/ci/pipeline.yml -l path/to/secrets.yml
```

If you want to use the defaults and have access to the continuous-integration
secrets, there is a convenience recipe:

```
make deploy-pipeline
```

Currently the secrets file is only being used to send notifications of failures
to a slack channel. If you wish to disable this, remove the reference to the
`slack-alert` anchor from the `unit-tests` job's `on_failure`.

To make the pipeline publicly visible, run:

```
fly --target [target-name] expose-pipeline --pipeline [pipeline-name]
```

(Similarly, there is a `make expose-pipeline` convenience recipe in the
Makefile.)

This will allow anyone to see the pipeline and its status. The details of the
run will not be visible unless the user is logged in to concourse.
