# Tests

<!-- TOC -->
- [Performance and Scale Testing](#performance-and-scale-testing)
- [Acceptance Tests](#acceptance-tests)
    - [Testing GPDB changes against gpupgrade](#testing-gpdb-changes-against-gpupgrade) 
    - [1) gpupgrade Acceptance Tests](#1-gpupgrade-acceptance-tests)
    - [2) pg_upgrade Acceptance Tests](#2-pg_upgrade-acceptance-tests)
        - [pg_upgrade non-upgradeable tests (negative tests)](#2a-pg_upgrade-non-upgradeable-tests-negative-tests)
        - [pg_upgrade: upgradeable tests (positive tests)](#2b-pg_upgrade-upgradeable-tests-positive-tests)
<!-- /TOC -->

---

# Performance and Scale Testing

Inside the perf_scale directory is a simple script to generate some data.

# Acceptance Tests

Acceptance tests are "end-to-end" tests that exercise the binary of gpupgrade and
the overall Greenplum upgrade process. There are two types of acceptance tests:
1) gpupgrade, and 2) pg_upgrade. And within the pg_upgrade acceptance tests
there are two sub-types: a) non-upgradeable, and b) upgradeable.

---

## Testing GPDB changes against gpupgrade

1. Each developer will need to sync the latest origin tags with their remote. This will allow the GPDB test rpm to have 
   the correct version number. For your GPDB branch run the following:

```
$ git fetch --tags origin
$ git push --tags <yourRemoteName>
```
*Note:* If you already flew a pipeline *before* pushing tags you will likely need to delete it, push tags, and re-fly as 
Concourse has some weird caching issues.

2. Fly a GPDB test pipeline to build a test release candidate RPM based on your branch using the `--build-test-rc` flag.  
Example gen_pipeline commands: 
- For 5X: `./gen_pipeline.py -t cm --build-test-rc -o /tmp/5X_rc.yml`
- For 6X: `./gen_pipeline.py -t cm --build-test-rc -O 'centos6' 'centos7' 'photon3' 'rhel8' -o /tmp/6X_rc.yml`
- For 7X: `./gen_pipeline.py -t cm --build-test-rc -O centos7 rhel8 -o /tmp/7X_rc.yml`

3. Create a gpupgrade test branch and push it. Next, generate a gpupgrade test pipeline that uses the GPDB RC RPMs using 
the appropriate environment variables:
`make 5X_GIT_USER=alice 5X_GIT_BRANCH=5X_rc 6X_GIT_USER=bob 6X_GIT_BRANCH=6X_rc pipeline`

*Note:* This will use the test RC RPM's for the rpm resources such as `gpdb6_centos7_rpm`. This will not use the GPDB RC 
test branch for the src resources such as `gpdb6_src` which will continue to use 6X_STABLE. This is expected since only 
the RPM's are needed for proper testing. 

---

## 1) gpupgrade Acceptance Tests

These tests exercise the end-to-end components and features of gpupgrade from the perspective of the gpupgrade binary.

Set the following environment variables:
- `PGPORT`: source cluster coordinator port
- `GPHOME_SOURCE`: The source cluster's installation path such as `/usr/local/gpdb5`.
- `GPHOME_TARGET`: The target cluster's installation path such as `/usr/local/gpdb6`.

And then run: `make acceptance`

---

## 2) pg_upgrade Acceptance Tests

The pg_upgrade tests make use of the pg_isolation2 framework, which is a GPDB
enhancement over the pg_regress framework. The tests comprise the following:
- A `.sql` file in the `sql/` directory which is the test source used by the
test runner.
- A `.out` file in the `expected/` directory called the "answer" file. This
records the expected output.
- The `schedule` file lists the tests to run. Entries on the same line are
run in parallel.

Notes:
1. Some tests require variable substitution. For example, `@abs_srcdir@` in 
`external_table` tests. For variable substitution, the isolation2 framework
requires  using a `.source` file in `input/`, and a `.source` file in `output/`.
When the test  is run the framework will generate the associated `.sql` and
`.out` files. Add the generated files to `sql/.gitignore` and
`expected/.gitignore`.

Note that this variable substitution is different from environment variable
substitution within a `!\` shell context such as:
`!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;`


2. Noteworthy isolation2 framework files (GPDB source repo):
* `atmsort.pl`: Documents the diffing logic used to compare the output against
the expected/answer file. It also explains the sort mechanism used to ignore
the order of results from SELECT queries. And explains matchers and
substitutions used to perform a "smart" diff.
* `pg_regress.c: convert_line`: Defines the variable substitutions such as `@abs_srcdir@`.
* `sql_isolation_testcase.py`: Class-level doc for `SQLIsolationTestCase`
 which explains the syntax extensions the isolation2 framework provides.

**Running pg_upgrade Acceptance Tests:**
1. Set the following environment variables:
- `PGPORT`: source cluster coordinator port
- `GPHOME_SOURCE`: The source cluster's installation path such as `/usr/local/gpdb5`.
- `GPHOME_TARGET`: The target cluster's installation path such as `/usr/local/gpdb6`.
- `ISOLATION2_PATH`: The path to the target gpdb version's pg_isolation2 binary
  such as `~/workspace/gpdb6/src/test/isolation2`.
2. Run the tests:
   * **Entire suite**: `make pg-upgrade-tests`
   * **Non-upgradeable tests**: `go test -count=1 -timeout 35m -v ./test/acceptance/pg_upgrade -run Test_PgUpgrade_NonUpgradeable_Tests`
   * **Upgradeable tests**: `go test -count=1 -timeout 35m -v ./test/acceptance/pg_upgrade -run Test_PgUpgrade_Upgradeable_Tests`
   * **Migratable tests**: `go test -count=1 -timeout 35m -v ./test/acceptance/pg_upgrade -run Test_PgUpgrade_Migratable_Tests`
   * **Focused pg_upgrade tests**: Set the environment variable
     `FOCUS_TESTS` to a space separated list of tests before running
     pg_upgrade tests. For instance:
     ```
     FOCUS_TESTS="partition_index view_owner" go test -count=1 -v ./test/acceptance/pg_upgrade -run Test_PgUpgrade_Migratable_Tests
     ```
### pg_upgrade: non-upgradeable tests (negative tests)

The pg_upgrade non-upgradeable acceptance tests are negative tests. They assert
that `pg_upgrade --check` correctly detects non-upgradeable objects. They also
provide a way to vet the workarounds we provide to customers to resolve such
bad objects.

The general work flow for each test is:
- Create the non-upgradeable objects and any related upgradeable objects.
- Run gpupgrade initialize which calls `pg_upgrade --check`.
- Assert that the non-upgradeable objects have been flagged. This can be
verified by looking at the .txt files generated by `pg_upgrade --check`.
- Resolve the non-upgradeable objects by performing the customer workarounds as
described in the [documentation](https://gpdb.docs.pivotal.io/upgrade/1-0/gpupgrade_initialize_checks.html).
Ensure tests are self-contained by resolving the non-upgradeable objects, and
enabling subsequent tests to not detect them.

Notes:
- **Only** drop the non-upgradeable objects and not the associated
upgradeable parts. For e.g., given a partition table with non-upgradeable indexes,
one only drops the indexes rather than the entire table. This enables the tests 
to be as close to the customer workflow as possible.
- As opposed to the postgres pg_regress tests these non-upgradeable tests are
*not* order dependent. Also, the non-upgradeable tests *cannot* be run in
parallel as opposed to upgradeable tests.
- The first test in the schedule will take longer than the subsquent tests since
they will skip many of the `gpupgrade initialize` substeps which were already run.
- Note, the pg_regress framework gives us smart diffs. Specifically, the output
from `SELECT` queries don't require an `ORDER BY` for deterministic output
comparison. See atmsort.pl in the gpdb repo for details. However, the framework
will *not* sort the output of shell commands such as `! cat .. ;`. Therefore, use
`! cat .. | LC_ALL=C sort -b;` to sort results independent of platforms. 
See [PR #555](https://github.com/greenplum-db/gpupgrade/pull/555) for details.

### pg_upgrade: upgradeable tests (positive tests)

The pg_upgrade upgradeable acceptance tests are positive tests. They assert
that interesting objects can be successfully upgraded by gpupgrade.

The upgradeable tests create the objects in the source cluster using
`source_cluster_regress/`, and assert they have been upgraded using
`target_cluster_regress/`.

The test framework executes the following steps:
- Runs all the tests in `source_cluster_regress/upgradeable_source_schedule`
to create all the upgradeable objects in the source cluster.
- Upgrades the cluster with `gpupgrade initialize` and `gpupgrade execute`.
- Runs all the tests in `target_cluster_regress/upgradeable_target_schedule` to
validate that all the upgradeable objects created in the source cluster have
been upgraded successfully in the target cluster.

Notes:
- The majority of the upgradeable tests *can* be run in  parallel as opposed to
non-upgradeable tests.

### pg_upgrade: migratable tests (positive tests)

The migratable acceptance tests are positive tests. They contain objects that are
not directly upgradable by pg_upgrade unless modified by migrations scripts.
This can include:
- Dropping non-upgradable objects before upgrade and recreating them post-upgrade.
- Modifying the object such as column type to a more upgrade friendly format.

The test framework executes the following steps:
- Runs all the tests in `source_cluster_regress/migratable_source_schedule`
to create all the upgradeable objects in the source cluster.
- Runs the `initialize` data migration scripts.
- Upgrades the cluster with `gpupgrade initialize`, `gpupgrade execute`, `gpupgrade finalize`.
- Runs the `finalize` data migration scripts.
- Runs all the tests in `target_cluster_regress/migratable_target_schedule` to
validate that all the migratable objects created in the source cluster have
been upgraded successfully in the target cluster.

Notes:
- The majority of the migratable tests *can* be run in parallel.
