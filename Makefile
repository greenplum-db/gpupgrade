all: build

.DEFAULT_GOAL := all
MODULE_NAME=gpupgrade
AGENT=gpupgrade_agent
CLI=gpupgrade
HUB=gpupgrade_hub

GIT_VERSION := $(shell git describe --tags | perl -pe 's/(.*)-([0-9]*)-(g[0-9a-f]*)/\1+dev.\2.\3/')
UPGRADE_VERSION_STR="-X $(MODULE_NAME)/cli/commanders.UpgradeVersion=$(GIT_VERSION)"

BRANCH := $(shell git for-each-ref --format='%(objectname) %(refname:short)' refs/heads | awk "/^$$(git rev-parse HEAD)/ {print \$$2}")
LINUX_PREFIX := env GOOS=linux GOARCH=amd64
MAC_PREFIX := env GOOS=darwin GOARCH=amd64
LINUX_POSTFIX := .linux.$(BRANCH)
MAC_POSTFIX := .darwin.$(BRANCH)

GOFLAGS := -gcflags="all=-N -l"

dependencies :
		go get -u github.com/golang/protobuf/protoc-gen-go
		go get golang.org/x/tools/cmd/goimports
		go get github.com/golang/lint/golint
		go get github.com/onsi/ginkgo/ginkgo
		go get github.com/alecthomas/gometalinter
		gometalinter --install
		go get github.com/golang/dep/cmd/dep
		dep ensure

depend : dependencies

format :
		goimports -w .

lint :
		! gofmt -l agent/ cli/ db/ helpers/ hub/ install/ integrations/ shellparsers/ testutils/ utils/ | read
		gometalinter --config=gometalinter.config -s vendor ./...

unit :
		ginkgo -r -keepGoing -randomizeSuites -randomizeAllSpecs --skipPackage=integrations

integration:
		ginkgo -r -keepGoing -randomizeAllSpecs integrations

# check runs all tests. Use -k to keep going after the first failure.
.PHONY: check
check:
		ginkgo -r -keepGoing -randomizeSuites -randomizeAllSpecs
		PATH=.:$$PATH bats -r ./test

test : lint unit integration

coverage :
		@./scripts/show_coverage.sh

sshd_build :
		make -C integrations/sshd

protobuf :
		protoc -I idl/ idl/*.proto --go_out=plugins=grpc:idl
		go get github.com/golang/mock/mockgen
		mockgen -source idl/cli_to_hub.pb.go  > mock_idl/cli_to_hub_mock.pb.go
		mockgen -source idl/hub_to_agent.pb.go  > mock_idl/hub_to_agent_mock.pb.go

PACKAGES := $(addsuffix -package,agent cli hub)
PREFIX = $($(OS)_PREFIX)
POSTFIX = $($(OS)_POSTFIX)

.PHONY: build build_linux build_mac $(PACKAGES)

build: $(PACKAGES)

build_linux: OS := LINUX
build_mac: OS := MAC
build_linux build_mac: build

agent-package: EXE_NAME := $(AGENT)
cli-package: EXE_NAME := $(CLI)
hub-package: EXE_NAME := $(HUB)

$(PACKAGES): %-package: .Gopkg.updated
	$(PREFIX) go build $(GOFLAGS) -o $(EXE_NAME)$(POSTFIX) -ldflags $(UPGRADE_VERSION_STR) github.com/greenplum-db/gpupgrade/$*

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

.PHONY: deploy-pipeline expose-pipeline

# You can override these two from the command line.
FLY_TARGET := gpdb-prod
PIPELINE_NAME := gpupgrade

deploy-pipeline:
	fly -t $(FLY_TARGET) set-pipeline -p $(PIPELINE_NAME) \
		-c ci/pipeline.yml \
		-l ~/workspace/continuous-integration/secrets/gpupgrade.dev.yml

expose-pipeline:
	fly --target $(FLY_TARGET) expose-pipeline --pipeline $(PIPELINE_NAME)
