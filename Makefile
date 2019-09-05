all: build

.DEFAULT_GOAL := all
MODULE_NAME=gpupgrade
AGENT=gpupgrade_agent
CLI=gpupgrade
HUB=gpupgrade_hub

# TAGGING
#   YOUR_BRANCH> make all of the changes you want for your tag
#   follow standard procedures for your PR; commit them(PR is completed)
#   note git hash for that version(might have been rebased, etc); call it GIT_HASH
#   YOUR_BRANCH> git tag -a TAGNAME -m "version 0.1.1: add version" GIT_HASH
#   YOUR_BRANCH> git push origin TAGNAME
GIT_VERSION := $(shell git describe --tags --long| perl -pe 's/(.*)-([0-9]*)-(g[0-9a-f]*)/\1+dev.\2.\3/')
VERSION_LD_STR="-X github.com/greenplum-db/$(MODULE_NAME)/utils.UpgradeVersion=$(GIT_VERSION)"

BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
LINUX_PREFIX := env GOOS=linux GOARCH=amd64
MAC_PREFIX := env GOOS=darwin GOARCH=amd64
LINUX_POSTFIX := .linux.$(BRANCH)
MAC_POSTFIX := .darwin.$(BRANCH)

GOFLAGS := -gcflags="all=-N -l"

.PHONY: depend depend-dev
depend:
		go get github.com/onsi/ginkgo/ginkgo
		go get github.com/golang/dep/cmd/dep
		dep ensure

depend-dev: depend
		go install ./vendor/github.com/golang/protobuf/protoc-gen-go
		go install ./vendor/github.com/golang/mock/mockgen
		go get golang.org/x/tools/cmd/goimports
		go get golang.org/x/lint/golint
		go get github.com/alecthomas/gometalinter
		gometalinter --install

# NOTE: goimports subsumes the standard formatting rules of gofmt, but gofmt is
#       more flexible(custom rules) so we leave it in for this reason.
format:
		goimports -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/
		gofmt -l -w agent/ cli/ db/ hub/ integrations/ testutils/ utils/


lint:
		gometalinter --config=gometalinter.config -s vendor ./...

unit:
		ginkgo -r -keepGoing -randomizeSuites -randomizeAllSpecs --skipPackage=integrations

integration:
		ginkgo -r -keepGoing -randomizeAllSpecs integrations

# check runs all tests. Use -k to keep going after the first failure.
.PHONY: check
check:
		ginkgo -r -keepGoing -randomizeSuites -randomizeAllSpecs
		PATH=.:$$PATH bats -r ./test

test: lint unit integration

.PHONY: coverage
coverage:
	@./scripts/show_coverage.sh

sshd_build:
		make -C integrations/sshd

protobuf:
		protoc -I idl/ idl/*.proto --go_out=plugins=grpc:idl
		mockgen -source idl/cli_to_hub.pb.go  > mock_idl/cli_to_hub_mock.pb.go
		mockgen -source idl/hub_to_agent.pb.go  > mock_idl/hub_to_agent_mock.pb.go

PACKAGES := $(addsuffix -package,agent cli hub)
PREFIX = $($(OS)_PREFIX)
POSTFIX = $($(OS)_POSTFIX)

.PHONY: build build_linux build_mac $(PACKAGES)

build: $(PACKAGES)
	go generate ./cli/bash

build_linux: OS := LINUX
build_mac: OS := MAC
build_linux build_mac: build

agent-package: EXE_NAME := $(AGENT)
cli-package: EXE_NAME := $(CLI)
hub-package: EXE_NAME := $(HUB)

$(PACKAGES): %-package: .Gopkg.updated
	$(PREFIX) go build $(GOFLAGS) -o $(EXE_NAME)$(POSTFIX) -ldflags $(VERSION_LD_STR) github.com/greenplum-db/gpupgrade/$*

install_agent: agent-package
		@psql -t -d template1 -c 'SELECT DISTINCT hostname FROM gp_segment_configuration WHERE content != -1' > /tmp/seg_hosts 2>/dev/null; \
		if [ $$? -eq 0 ]; then \
			gpscp -f /tmp/seg_hosts $(AGENT) =:$(GPHOME)/bin/$(AGENT); \
			if [ $$? -eq 0 ]; then \
				echo 'Successfully copied gpupgrade_agent to $(GPHOME) on all segments'; \
			else \
				echo 'Failed to copy gpupgrade_agent to $(GPHOME)'; \
				exit 1; \
			fi; \
		else \
			echo 'Database is not running, please start the database and run this make target again'; \
			exit 1; \
		fi; \
		rm /tmp/seg_hosts

install: cli-package hub-package install_agent
		cp -p $(CLI) $(GPHOME)/bin/$(CLI)
		cp -p $(HUB) $(GPHOME)/bin/$(HUB)

# We intentionally do not depend on install here -- the point of installcheck is
# to test whatever has already been installed.
installcheck:
		@echo "--------------------------------------------------------------"
		@echo "# FIXME: Make, if run in parallel, hangs after test completes."
		./installcheck.bats

clean:
		# Build artifacts
		rm -f $(AGENT)
		rm -f $(CLI)
		rm -f $(HUB)
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
FLY_TARGET ?= dev
ifeq ($(FLY_TARGET),prod)
SECRETS_TYPE := prod
else
SECRETS_TYPE := dev
endif

.PHONY: set-pipeline expose-pipeline
# TODO: Keep this in sync with the README at github.com/greenplum-db/continuous-integration
set-pipeline:
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
