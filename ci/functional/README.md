# Functional Testing Pipeline

<!-- TOC -->
- [How to Use](#how-to-use)
  - [Create a gpupgrade branch](#create-a-gpupgrade-branch)
  - [Generating a Dump](#generating-a-dump)
  - [Using a Dump](#using-a-dump)
  - [CCP Cluster Settings](#ccp-cluster-settings)
  - [Flying the Pipeline](#flying-the-pipeline)
  - [Fixing Failures](#fixing-failures)
  - [Tearing Down the Cluster](#tearing-down-the-cluster)
- [Purpose](#purpose)
- [Design and Implementation](#design-and-implementation)
<!-- /TOC -->

---

# How to Use

#### Create a gpupgrade branch
- Create a gpupgrade branch without any private information.

#### Generating a Dump
- On a production cluster use `gpbackup --metadata-only --dbname <db>` for all databases to generate a schema only metadata dump.
- XZ the dump with `xz --threads $(nproc) customer_5X_metadata_05_2023.sql` to create a `customer_5X_metadata_05_2023.sql.xz` file.
- Place the xz'd dump in the `user-schemas` bucket under the `data-gpdb-server` GCP project.

#### Using a Dump
- Place _any_ xz'd SQL file in `gpupgrade-intermediates/dump/5X` bucket under the `data-gpdb-cm` GCP project.
- If multiple `.sql.xz` files are present in the bucket update the `schema_dump` resource to either use a more precise 
  regex or switch to using a `versioned_file` resource.

#### CCP Cluster Settings
The defaults should be fine. One can change the `instance_type`, `disk_type`, `disk_size`, and `ccp_reap_minutes` in the 
generate-cluster job.

#### Flying the Pipeline
Run `make functional-pipeline` to fly the pipeline

#### Fixing Failures
- Fix the change in the pipeline yaml and re-fly the pipeline.
- Log into the box and fix the issue and re-trigger the job.

#### Tearing Down the Cluster
- The end of the pipeline will run the `teardown-cluster` job to destroy the cluster.
- If the pipeline does not finish run `manually-destroy-cluster` to clean up the cluster.
- Note: If either of these jobs fail to run, CCP does eventually clean up the cluster based on `ccp_reap_minutes` and the 
  `instance creation_timestamp`. See comment in the `generate-cluster` job.

# Purpose

The functional pipeline easily tests user-schemas and any other SQL dump file. It spins up a performant CCP cluster, 
loads the SQL dump, and performs the upgrade.

The pipeline is designed to easily fix and re-run failed jobs without needing to reload the SQL dump saving many hours.
For example, if initialize fails on a pg_upgrade check, one can fix the issue and re-run the job without needing to 
reload the data.

# Design and Implementation

There are some design considerations since each step in the upgrade process is its own job.

We tar up and save the `cluster_env_files` that are produced from the generated CCP cluster. This `saved_cluster_env_files` 
resource is passed to each job since it has the information needed to connect to the cluster.

We do not place a passed constraint on `gpupgrade_src` or `saved_cluster_env_files` to easily push new changes to these 
resources. Without a passed constraint the job can be re-triggered and not fail with `fatal: reference is not a tree: error.`
This occurs when the commit history has been overwritten by a force-push and the job cannot find the correct SHA.

Since we don't place a passed constraint on `gpupgrade_src` or `saved_cluster_env_files` we use a `dummy_resource` to 
automatically trigger subsequent jobs during the upgrade workflow. 

The `saved_cluster_env_files` and `dummy_resource` include the branch name to avoid collisions when multiple pipelines are 
run.

To clean up the generated cluster we have a job at the end of the pipeline to teardown the cluster from the passed
terraform resource. Additionally, we have a job to manually destroy the cluster if the pipeline does not complete and
the cluster needs to be removed. If either of these jobs fail to run, CCP does eventually clean up the cluster based on
`ccp_reap_minutes` and the instance `creation_timestamp`. See comment in the `generate-cluster` job.
