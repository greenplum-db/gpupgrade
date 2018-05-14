.PHONY: all deploy deploy4to5 deploy4to6 deploy5to6

# You can override the fly target and/or pipeline on the command line.
TARGET=dev
PIPELINE_NAME=gpupgrade_multinode

# Set DEBUG to something non-empty during deployment to get really verbose
# testing output.
DEBUG=

all:

#
# Target-specific Variables
#
deploy4to5 deploy4to6: GPDB_BINARY_OLD=bin_gpdb4_centos6
deploy5to6: GPDB_BINARY_OLD=bin_gpdb5_centos6
deploy6to6: GPDB_BINARY_OLD=bin_gpdb6_centos6
deploy4to5: GPDB_BINARY_NEW=bin_gpdb5_centos6
deploy4to6 deploy5to6 deploy6to6: GPDB_BINARY_NEW=bin_gpdb6_centos6

deploy4to5: PIPELINE_NAME := "${PIPELINE_NAME}_4to5"
deploy4to6: PIPELINE_NAME := "${PIPELINE_NAME}_4to6"
deploy5to6: PIPELINE_NAME := "${PIPELINE_NAME}_5to6"
deploy6to6: PIPELINE_NAME := "${PIPELINE_NAME}_6to6"

# The various deploy targets will automatically point the pipeline to the current gpupgrade branch
BRANCH := $(shell git for-each-ref --format='%(objectname) %(refname:short)' refs/heads | awk "/^$$(git rev-parse HEAD)/ {print \$$2}")

# Alias for deploy6to6.
deploy: deploy6to6

deploy4to5 deploy4to6 deploy5to6 deploy6to6: gpupgrade_pipeline.yml
	fly -t $(TARGET) set-pipeline -p $(PIPELINE_NAME) -c $< \
	-l ~/workspace/continuous-integration/secrets/gpdb-perf-secrets.dev.yml \
	-l ~/workspace/continuous-integration/secrets/gpdb_common-ci-secrets.yml \
	-l ~/workspace/continuous-integration/secrets/gpdb_master_without_asserts-ci-secrets.yml \
	-l ~/workspace/continuous-integration/secrets/ccp_ci_secrets_gpdb-dev.yml \
	-v initial-cluster-gpdb-binary=$(GPDB_BINARY_OLD) \
	-v upgraded-cluster-gpdb-binary=$(GPDB_BINARY_NEW) \
	-v enable-debug-output=$(DEBUG) \
	-v gpupgrade-git-branch=$(BRANCH)
