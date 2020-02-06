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

.PHONY: depend depend-dev
depend:
		go get github.com/onsi/ginkgo/ginkgo
		go get github.com/golang/dep/cmd/dep
		dep ensure

depend-dev: depend
		go install ./vendor/github.com/golang/protobuf/protoc-gen-go
		go install ./vendor/github.com/golang/mock/mockgen
		go get golang.org/x/tools/cmd/goimports

# NOTE: goimports subsumes the standard formatting rules of gofmt, but gofmt is
#       more flexible(custom rules) so we leave it in for this reason.
format:
		goimports -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/
		gofmt -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/


.PHONY: check check-ginkgo check-bats unit integration test

# check runs all tests against the locally built gpupgrade binaries. Use -k to
# continue after failures.
check: check-ginkgo check-bats
check-ginkgo check-bats: export PATH := $(CURDIR):$(PATH)

GINKGO_ARGS := -keepGoing -randomizeSuites -randomizeAllSpecs
check-ginkgo:
	ginkgo -r $(GINKGO_ARGS)

check-bats:
	bats -r ./test

unit: GINKGO_ARGS += --skipPackage=integrations
unit: check-ginkgo

integration: GINKGO_ARGS += integrations
integration: check-ginkgo

test: unit integration

.PHONY: coverage
coverage:
	@./scripts/show_coverage.sh

sshd_build:
		make -C integrations/sshd

BUILD_ENV = $($(OS)_ENV)
EXTENSION = $($(OS)_EXTENSION)

.PHONY: build build_linux build_mac

build: .Gopkg.updated
	$(BUILD_ENV) go build -o gpupgrade$(EXTENSION) $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade
	go generate ./cli/bash

build_linux: OS := LINUX
build_mac: OS := MAC
build_linux build_mac: build

BUILD_FLAGS = -gcflags="all=-N -l"
override BUILD_FLAGS += -ldflags $(VERSION_LD_STR)

install: .Gopkg.updated
	go install $(BUILD_FLAGS) github.com/greenplum-db/gpupgrade/cmd/gpupgrade

# We intentionally do not depend on install here -- the point of installcheck is
# to test whatever has already been installed.
installcheck:
		@echo "--------------------------------------------------------------"
		@echo "# FIXME: Make, if run in parallel, hangs after test completes."
		./installcheck.bats

clean:
		# Build artifacts
		rm -f gpupgrade
		# Test artifacts
		rm -rf /tmp/go-build*
		rm -rf /tmp/gexec_artifacts*
		rm -rf /tmp/ginkgo*
		# Code coverage files
		rm -rf /tmp/cover*
		rm -rf /tmp/unit*

# This is a manual marker file to track the last time we ran `dep ensure`
# locally, compared to the timestamps of the Gopkg.* metafiles. Define a
# dependency on this marker to run a `dep ensure` (if necessary) before your
# recipe is run.
.Gopkg.updated: Gopkg.lock Gopkg.toml
	dep ensure
	touch $@

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
SECRETS_TYPE := prod
else
SECRETS_TYPE := dev
endif

.PHONY: set-pipeline expose-pipeline
# TODO: Keep this in sync with the README at github.com/greenplum-db/continuous-integration
set-pipeline:
	# Keep pipeline.yml up to date
	go generate ./ci
	#NOTE-- make sure your gpupgrade-git-remote uses an https style git"
	#NOTE-- such as https://github.com/greenplum-db/gpupgrade.git"
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/pipeline.yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpupgrade.$(SECRETS_TYPE).yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpdb_common-ci-secrets.yml \
		-l ~/workspace/gp-continuous-integration/secrets/gpdb_master-ci-secrets.prod.yml \
		-l ~/workspace/gp-continuous-integration/secrets/ccp_ci_secrets_$(FLY_TARGET).yml \
		-v gpupgrade-git-remote=$(GIT_URI) \
		-v gpupgrade-git-branch=$(BRANCH)

expose-pipeline:
	fly --target $(FLY_TARGET) expose-pipeline --pipeline $(PIPELINE_NAME)
