# gpupgrade ci pipeline

This document is for developers when they consider changes to the pipeline. It
is currently not complete, but sections will be added as need be.

## SOURCE and TARGET clusters for our tests
The SOURCE and TARGET versions are currently hand-curated.  Note that two rpm files need to be updated: `rhel6` and `rhel7`.

We pull a specific version of the
rpms from [Pivnet](https://network.pivotal.io/products/pivotal-gpdb/#/releases/683946/file_groups/2659) or 
from its corresponding gcp instance path and place it ourselves into both the _prod_
and _dev_ buckets.  You do this via downloading the rpm files from [Pivnet](https://network.pivotal.io/products/pivotal-gpdb/#/releases/683946/file_groups/2659)
or gcp:

`gsutil cp gs://pivotal-gpdb-concourse-resources-prod/server/released/gpdb6/greenplum-db-6.9.0-rhel7-x86_64.rpm localDir`

`gsutil cp gs://pivotal-gpdb-concourse-resources-prod/server/released/gpdb6/greenplum-db-6.9.0-rhel6-x86_64.rpm localDir`


and then using the upload feature on the GUI here to upload the files:

- [gpugprade-artifacts-dev](https://console.cloud.google.com/storage/browser/gpupgrade-artifacts-dev?forceOnBucketsSortingFiltering=false&project=data-gpdb-cm)
  - This is used for the cm dev instance.
  - Note that as soon as you drop a new release here, any triggered pipeline in cm will automatically pull the latest semver
  of the rpm.
- [gpupgrade-artifacts-prod](https://console.cloud.google.com/storage/browser/gpupgrade-artifacts-prod?forceOnBucketsSortingFiltering=false&project=data-gpdb-cm
)
  - This is used for the prod instance.  
  - Note that as soon as you drop a new release here, any triggered pipeline in prod will automatically pull the latest semver
  of the rpm.
  
The _dev_ and _prod_ buckets each have two subdirectories: "greenplum-db-5" and "greenplum-db-6".  You copy the
corresponding versions(5/6) into the corresponding directory.  The changes are automatically picked up by each 
pipeline when it is triggered.

The recommended workflow:
1. copy in the new 5 or 6(or both) versions into the appropriate directory on [gpugprade-artifacts-dev](https://console.cloud.google.com/storage/browser/gpupgrade-artifacts-dev?forceOnBucketsSortingFiltering=false&project=data-gpdb-cm)
1. fly a test pipeline in `cm` and make sure it passes.  Note you can use `fly trigger-job` to restart jobs in the pipeline.
1. copy in a new 5 or 6(or both) versions into the appropriate directory on [gpupgrade-artifacts-prod](https://console.cloud.google.com/storage/browser/gpupgrade-artifacts-prod?forceOnBucketsSortingFiltering=false&project=data-gpdb-cm)
1. fly the `prod` pipeline and make sure it passes.




