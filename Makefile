# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
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
	go install github.com/golang/protobuf/protoc-gen-go
	go install github.com/golang/mock/mockgen

# NOTE: goimports subsumes the standard formatting rules of gofmt, but gofmt is
#       more flexible(custom rules) so we leave it in for this reason.
format:
		goimports -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/
		gofmt -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/


.PHONY: check check-go gpupgrade-tests unit integration test

# check runs all tests against the locally built gpupgrade binaries. Use -k to
# continue after failures.
check: check-go gpupgrade-tests
check-go gpupgrade-tests: export PATH := $(CURDIR):$(PATH)

TEST_PACKAGES := ./...

# FIXME go test currently caches integration tests incorrectly, because we do
# not register any dependency on the gpupgrade binary that we rely on for
# testing. For now, disable test caching for the Make recipes with -count=1;
# anyone who would like caching back can always use `go test` directly.
check-go:
	go test -count=1 $(TEST_PACKAGES)

gpupgrade-tests:
	bats -r ./test/acceptance/gpupgrade

pg-upgrade-tests:
	bats -r ./test/acceptance/pg_upgrade

unit: TEST_PACKAGES := $(shell go list ./... | grep -v integrations$$ )
unit: check-go

integration: TEST_PACKAGES := ./integrations
integration: check-go

test: unit integration

.PHONY: coverage
coverage:
	@./scripts/show_coverage.sh

sshd_build:
		make -C integrations/sshd

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
	rm -r tarball/data-migration-scripts/test
	# create tarball
	( cd tarball; tar czf ../$(TARBALL_NAME) . )
	sha256sum $(TARBALL_NAME) > CHECKSUM
	rm -r tarball

enterprise-rpm: RELEASE=Enterprise
enterprise-rpm: NAME=VMware Tanzu Greenplum Upgrade
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
ifeq ($(BRANCH),master)
	PIPELINE_NAME := gpupgrade
	FLY_TARGET := prod
endif
endif

# Concourse does not allow "/" in pipeline names
WORKSPACE ?= ~/workspace
PIPELINE_NAME ?= gpupgrade:$(shell git rev-parse --abbrev-ref HEAD | tr '/' ':')
FLY_TARGET ?= cm
ifeq ($(FLY_TARGET),prod)
	TARGET := prod
else
	TARGET := dev
endif

.PHONY: set-pipeline expose-pipeline
set-pipeline: export 5X_GIT_USER=${5X_GIT_USER:-}
set-pipeline: export 5X_GIT_BRANCH=${5X_GIT_BRANCH:-}
set-pipeline: export 6X_GIT_USER=${6X_GIT_USER:-}
set-pipeline: export 6X_GIT_BRANCH=${6X_GIT_BRANCH:-}
set-pipeline: export 7X_GIT_USER=${7X_GIT_USER:-}
set-pipeline: export 7X_GIT_BRANCH=${7X_GIT_BRANCH:-}
set-pipeline:
	mkdir -p ci/generated
	cat ci/1_resources_anchors_groups.yml \
		ci/2_build_lint.yml \
		ci/3_gpupgrade_jobs.yml  \
		ci/4_pg_upgrade_jobs.yml  \
		ci/5_multi_host_gpupgrade_jobs.yml \
		ci/6_upgrade_and_functional_jobs.yml \
		ci/7_publish_rc.yml > ci/generated/template.yml
	go generate ./ci
	#NOTE-- make sure your gpupgrade-git-remote uses an https style git"
	#NOTE-- such as https://github.com/greenplum-db/gpupgrade.git"
	# TODO: Keep this in sync with the README at github.com/greenplum-db/continuous-integration
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/generated/pipeline.yml \
		-l $(WORKSPACE)/gp-continuous-integration/secrets/gpupgrade.$(TARGET).yml \
		-l $(WORKSPACE)/gp-continuous-integration/secrets/gpdb_common-ci-secrets.yml \
		-l $(WORKSPACE)/gp-continuous-integration/secrets/gpdb_master-ci-secrets.$(TARGET).yml \
		-l $(WORKSPACE)/gp-continuous-integration/secrets/ccp_ci_secrets_$(FLY_TARGET).yml \
		-l $(WORKSPACE)/gp-continuous-integration/secrets/gp-upgrade-packaging.dev.yml \
		-v gpupgrade-git-remote=$(GIT_URI) \
		-v gpupgrade-git-branch=$(BRANCH)

expose-pipeline:
	fly --target $(FLY_TARGET) expose-pipeline --pipeline $(PIPELINE_NAME)
