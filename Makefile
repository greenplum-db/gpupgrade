all: build

.DEFAULT_GOAL := all
MODULE_NAME=gpupgrade

# TAGGING
#   YOUR_BRANCH> make all of the changes you want for your tag
#   follow standard procedures for your PR; commit them(PR is completed)
#   note git hash for that version(might have been rebased, etc); call it GIT_HASH
#   YOUR_BRANCH> git tag -a TAGNAME -m "version 0.1.1: add version" GIT_HASH
#   YOUR_BRANCH> git push origin TAGNAME
GIT_VERSION := $(shell git describe --tags --long| perl -pe 's/(.*)-([0-9]*)-(g[0-9a-f]*)/\1+dev.\2.\3/')
VERSION_LD_STR="-X github.com/greenplum-db/$(MODULE_NAME)/cli/commands.UpgradeVersion=$(GIT_VERSION)"

BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
LINUX_ENV := env GOOS=linux GOARCH=amd64
MAC_ENV := env GOOS=darwin GOARCH=amd64
LINUX_EXTENSION := .linux.$(BRANCH)
MAC_EXTENSION := .darwin.$(BRANCH)

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


.PHONY: check check-go check-bats unit integration test

# check runs all tests against the locally built gpupgrade binaries. Use -k to
# continue after failures.
check: check-go check-bats
check-go check-bats: export PATH := $(CURDIR):$(PATH)

TEST_PACKAGES := ./...

# FIXME go test currently caches integration tests incorrectly, because we do
# not register any dependency on the gpupgrade binary that we rely on for
# testing. For now, disable test caching for the Make recipes with -count=1;
# anyone who would like caching back can always use `go test` directly.
check-go:
	go test -count=1 $(TEST_PACKAGES)

check-bats:
	bats -r ./test

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
EXTENSION = $($(OS)_EXTENSION)

.PHONY: build build_linux build_mac

build:
	$(BUILD_ENV) go build -o gpupgrade$(EXTENSION) $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade
	go generate ./cli/bash

build_linux: OS := LINUX
build_mac: OS := MAC
build_linux build_mac: build

BUILD_FLAGS = -gcflags="all=-N -l"
override BUILD_FLAGS += -ldflags $(VERSION_LD_STR)

install:
	go install $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade

# We intentionally do not depend on install here -- the point of installcheck is
# to test whatever has already been installed.
installcheck:
		@echo "--------------------------------------------------------------"
		@echo "# FIXME: Make, if run in parallel, hangs after test completes."
		./installcheck.bats

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

# You can override these from the command line.
GIT_URI := $(shell git ls-remote --get-url)

ifeq ($(GIT_URI),https://github.com/greenplum-db/gpupgrade)
ifeq ($(BRANCH),master)
	PIPELINE_NAME := gpupgrade
	FLY_TARGET := prod
endif
endif

# Concourse does not allow "/" in pipeline names
PIPELINE_NAME ?= gpupgrade:$(shell git rev-parse --abbrev-ref HEAD | tr '/' ':')
FLY_TARGET ?= cm
ifeq ($(FLY_TARGET),prod)
	TARGET := prod
else
	TARGET := dev
endif

.PHONY: set-pipeline expose-pipeline
# TODO: Keep this in sync with the README at github.com/greenplum-db/continuous-integration
set-pipeline:
	# Keep pipeline.yml up to date
	go generate ./ci
	#NOTE-- make sure your gpupgrade-git-remote uses an https style git"
	#NOTE-- such as https://github.com/greenplum-db/gpupgrade.git"
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/generated/pipeline.yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpupgrade.$(TARGET).yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpdb_common-ci-secrets.yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpdb_master-ci-secrets.prod.yml \
		-l ~/workspace/gp-continuous-integration/secrets/ccp_ci_secrets_$(FLY_TARGET).yml \
		-l ~/workspace/gp-continuous-integration/secrets/gp-upgrade-packaging.dev.yml \
		-v gpupgrade-git-remote=$(GIT_URI) \
		-v gpupgrade-git-branch=$(BRANCH)

expose-pipeline:
	fly --target $(FLY_TARGET) expose-pipeline --pipeline $(PIPELINE_NAME)
