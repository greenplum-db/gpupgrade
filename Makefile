# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

all: build

.DEFAULT_GOAL := all
MODULE_NAME=gpupgrade


LINUX_ENV := env GOOS=linux GOARCH=amd64
MAC_ENV := env GOOS=darwin GOARCH=amd64

# depend-dev will install the necessary Go dependencies for running `go
# generate`. (This recipe does not have to be run in order to build the
# project; only to rebuild generated files.) Note that developers must still
# install the protoc compiler themselves; there is no way to version it from
# within the Go module system.
#
# Though it's a little counter-intuitive, run this recipe AFTER running make for
# the first time, so that Go will have already fetched the packages that are
# pinned in tools.go.
.PHONY: depend-dev
depend-dev: export GOBIN := $(CURDIR)/dev-bin
depend-dev: export GOFLAGS := -mod=readonly # do not update dependencies during installation
depend-dev:
	mkdir -p $(GOBIN)
	go install github.com/golang/protobuf/protoc-gen-go@v1.3.2
	go install github.com/golang/mock/mockgen

# NOTE: goimports subsumes the standard formatting rules of gofmt, but gofmt is
#       more flexible(custom rules) so we leave it in for this reason.
format:
		goimports -l -w agent/ cli/ db/ hub/ integration/ testutils/ utils/
		gofmt -l -w agent/ cli/ db/ hub/ integration/ testutils/ utils/

unit integration acceptance test: export PATH := $(CURDIR):$(PATH)

.PHONY: unit
unit:
	go test -count=1 $(shell go list ./... | grep -v integration$$ )

.PHONY: integration
integration:
	go test -count=1 ./integration

.PHONY: acceptance
acceptance:
	bats -r ./test/acceptance/gpupgrade

# test runs all tests against the locally built gpupgrade binaries. Use -k to
# continue after failures.
.PHONY: test check
test check: unit integration acceptance

.PHONY: pg-upgrade-tests
pg-upgrade-tests:
	bats -r ./test/acceptance/pg_upgrade

.PHONY: coverage
coverage:
	@./scripts/show_coverage.sh

BUILD_ENV = $($(OS)_ENV)

.PHONY: build build_linux build_mac

build:
	# For tagging a release see the "Upgrade Release Checklist" document.
	$(eval VERSION := $(shell git describe --tags --abbrev=0))
	$(eval COMMIT := $(shell git rev-parse --short --verify HEAD))
	$(eval RELEASE=Dev Build)
	$(eval VERSION_LD_STR := -X 'github.com/greenplum-db/$(MODULE_NAME)/cli/commands.Version=$(VERSION)')
	$(eval VERSION_LD_STR += -X 'github.com/greenplum-db/$(MODULE_NAME)/cli/commands.Commit=$(COMMIT)')
	$(eval VERSION_LD_STR += -X 'github.com/greenplum-db/$(MODULE_NAME)/cli/commands.Release=$(RELEASE)')

	$(eval BUILD_FLAGS = -gcflags="all=-N -l")
	$(eval override BUILD_FLAGS += -ldflags "$(VERSION_LD_STR)")

	$(BUILD_ENV) go build -o gpupgrade $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade
	go generate ./cli/bash

build_linux: OS := LINUX
build_mac: OS := MAC
build_linux build_mac: build

BUILD_FLAGS = -gcflags="all=-N -l"
override BUILD_FLAGS += -ldflags "$(VERSION_LD_STR)"

enterprise-tarball: RELEASE=Enterprise
enterprise-tarball: build tarball

oss-tarball: RELEASE=Open Source
oss-tarball: build tarball

TARBALL_NAME=gpupgrade.tar.gz

tarball:
	[ ! -d tarball ] && mkdir tarball
	# gather files
	cp gpupgrade tarball
	cp cli/bash/gpupgrade.bash tarball
	cp gpupgrade_config tarball
	cp open_source_licenses.txt tarball
	cp -r data-migration-scripts/ tarball/data-migration-scripts/
	# remove test files
	rm -r tarball/data-migration-scripts/5-to-6-seed-scripts/test
	# create tarball
	( cd tarball; tar czf ../$(TARBALL_NAME) . )
	sha256sum $(TARBALL_NAME) > CHECKSUM
	rm -r tarball

enterprise-rpm: RELEASE=Enterprise
enterprise-rpm: NAME=VMware Greenplum Upgrade
enterprise-rpm: LICENSE=VMware Software EULA
enterprise-rpm: enterprise-tarball rpm

oss-rpm: RELEASE=Open Source
oss-rpm: NAME=Greenplum Database Upgrade
oss-rpm: LICENSE=Apache 2.0
oss-rpm: oss-tarball rpm

rpm:
	[ ! -d rpm ] && mkdir rpm
	mkdir -p rpm/rpmbuild/{BUILD,RPMS,SOURCES,SPECS}
	cp $(TARBALL_NAME) rpm/rpmbuild/SOURCES
	cp gpupgrade.spec rpm/rpmbuild/SPECS/
	rpmbuild \
	--define "_topdir $${PWD}/rpm/rpmbuild" \
	--define "gpupgrade_version $(VERSION)" \
	--define "gpupgrade_rpm_release 1" \
	--define "release_type $(RELEASE)" \
	--define "license $(LICENSE)" \
	--define "summary $(NAME)" \
	-bb $${PWD}/rpm/rpmbuild/SPECS/gpupgrade.spec
	cp rpm/rpmbuild/RPMS/x86_64/gpupgrade-$(VERSION)*.rpm .
	rm -r rpm

install:
	go install $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade

# To lint, you must install golangci-lint via one of the supported methods
# listed at
#
#     https://github.com/golangci/golangci-lint#install
#
# DO NOT add the linter to the project dependencies in Gopkg.toml, as much as
# you may want to streamline this installation process, because
# 1. `go get` is an explicitly unsupported installation method for this utility,
#    much like it is for gpupgrade itself, and
# 2. adding it as a project dependency opens up the possibility of accidentally
#    vendoring GPL'd code.
.PHONY: lint
lint:
	golangci-lint run

clean:
		# Build artifacts
		rm -f gpupgrade
		# Test artifacts
		rm -rf /tmp/go-build*
		rm -rf /tmp/gexec_artifacts*
		# Code coverage files
		rm -rf /tmp/cover*
		rm -rf /tmp/unit*
		# Package artifacts
		rm -rf tarball
		rm -f $(TARBALL_NAME)
		rm -f CHECKSUM
		rm -rf rpm
		rm -f gpupgrade-$(VERSION)*.rpm

# You can override these from the command line.
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
GIT_URI ?= $(shell git ls-remote --get-url)

ifeq ($(GIT_URI),https://github.com/greenplum-db/gpupgrade.git)
ifeq ($(BRANCH),main)
	PIPELINE_NAME := gpupgrade
	FLY_TARGET := prod
endif
endif

# Concourse does not allow "/" in pipeline names
WORKSPACE ?= ~/workspace
BRANCH_NAME ?= $(shell git rev-parse --abbrev-ref HEAD | tr '/' ':')
export BRANCH_NAME
PIPELINE_NAME ?= gpupgrade:${BRANCH_NAME}
FLY_TARGET ?= cm

# YAML templating is used to switch between prod and dev pipelines. The
# environment variable JOB_TYPE is used to determine whether a dev or prod
# pipeline is generated. It is used when go generate runs our yaml parser.
ifeq ($(FLY_TARGET),prod)
pipeline functional-pipeline: export JOB_TYPE=prod
else
pipeline functional-pipeline: export JOB_TYPE=dev
endif

.PHONY: pipeline functional-pipeline expose-pipeline
pipeline functional-pipeline: export DUMP_PATH=${DUMP_PATH:-}
pipeline functional-pipeline: export 5X_GIT_USER=${5X_GIT_USER:-}
pipeline functional-pipeline: export 5X_GIT_BRANCH=${5X_GIT_BRANCH:-}
pipeline functional-pipeline: export 6X_GIT_USER=${6X_GIT_USER:-}
pipeline functional-pipeline: export 6X_GIT_BRANCH=${6X_GIT_BRANCH:-}
pipeline functional-pipeline: export 7X_GIT_USER=${7X_GIT_USER:-}
pipeline functional-pipeline: export 7X_GIT_BRANCH=${7X_GIT_BRANCH:-}
pipeline:
	mkdir -p ci/main/generated
	cat ci/main/pipeline/1_resources_anchors_groups.yml \
		ci/main/pipeline/2_build_lint.yml \
		ci/main/pipeline/3_gpupgrade_jobs.yml  \
		ci/main/pipeline/4_pg_upgrade_jobs.yml  \
		ci/main/pipeline/5_multi_host_gpupgrade_jobs.yml \
		ci/main/pipeline/6_upgrade_and_functional_jobs.yml \
		ci/main/pipeline/7_publish_rc.yml > ci/main/generated/template.yml
	go generate ./ci/main
	#NOTE-- make sure your gpupgrade-git-remote uses an https style git"
	#NOTE-- such as https://github.com/greenplum-db/gpupgrade.git"
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/main/generated/pipeline.yml \
		-v gpupgrade-git-remote=$(GIT_URI) \
		-v gpupgrade-git-branch=$(BRANCH)

functional-pipeline:
	mkdir -p ci/functional/generated
	cat ci/functional/pipeline/1_resources_anchors_groups.yml \
		ci/functional/pipeline/2_generate_cluster.yml \
		ci/functional/pipeline/3_load_schema_data_migration_scripts.yml \
		ci/functional/pipeline/4_initialize_upgrade_cluster_validate.yml \
		ci/functional/pipeline/5_teardown_cluster.yml > ci/functional/generated/template.yml
	go generate ./ci/functional
	#NOTE-- make sure your gpupgrade-git-remote uses an https style git"
	#NOTE-- such as https://github.com/greenplum-db/gpupgrade.git"
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/functional/generated/pipeline.yml \
		-v gpupgrade-git-remote=$(GIT_URI) \
		-v gpupgrade-git-branch=$(BRANCH)

expose-pipeline:
	fly --target $(FLY_TARGET) expose-pipeline --pipeline $(PIPELINE_NAME)
