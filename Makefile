ROOT              := $(realpath $(dir $(realpath $(firstword $(MAKEFILE_LIST)))))
comma             := ,

# some systems requires opt-in for buildx
DOCKER_BUILDKIT   := 1
export DOCKER_BUILDKIT

ifdef CI
  BOLD  :=
  CYAN  :=
  RESET :=
else
  BOLD  := \033[1m
  CYAN  := \033[36m
  RESET := \033[0m
endif

BANNER = @printf "$(BOLD)$(CYAN)[target: $@]$(RESET)\n"

# Allocate a TTY in dev (for ctrl+c) but not in CI
MK_DOCKER_RUN_OPTS_TTY := $(if $(CI),,-it)
export MK_DOCKER_RUN_OPTS_TTY


# Safely detect a unique system identifier into a variable
MK_SYSTEM_ID := $(strip $(shell \
    if [ -s /etc/machine-id ]; then \
        cat /etc/machine-id 2>/dev/null; \
    elif command -v hostname >/dev/null 2>&1; then \
        hostname 2>/dev/null; \
    else \
        echo -n "unknown"; \
    fi))

# User might have several repos in a host. Distinguish each by using the abs path of the repo
MK_REPO_ID                := $(shell printf '%s' "$(ROOT)$(MK_SYSTEM_ID)" | sha256sum | cut -c1-8)
MK_DOCKER_PROGRESS        ?= plain
MK_DOCKER_PULL            ?= --pull
MK_TEST_INTEGRATION_IMAGE := seeder-test-integration:$(MK_REPO_ID)

# Legacy dapper env variables
REPO                      ?=
PUSH                      ?=
DRONE_BRANCH              ?=
DRONE_TAG                 ?=

export MK_DOCKER_PROGRESS MK_DOCKER_PULL MK_REPO_ID
export REPO PUSH DRONE_BRANCH DRONE_TAG

MK_HOST_ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
ARCH := $(MK_HOST_ARCH)
export MK_HOST_ARCH
export ARCH

DOCKER_BUILD = docker build $(MK_DOCKER_PULL) \
	--progress=$(MK_DOCKER_PROGRESS) \
	--build-arg MK_REPO_ID \
	--build-arg MK_HOST_ARCH \
	-f $(ROOT)/Dockerfile $(ROOT)

.PHONY: build ci generate package test validate


# ---- Directories ----
$(ROOT)/bin:
	@mkdir -p $@


# ---- Pre-generate version env for container builds (no .git needed inside Docker) ----
# Also handles git worktree checkouts where .git is a pointer file to an external directory.
gen-version-env:
	$(BANNER)
	@bash $(ROOT)/scripts/version > /dev/null


# ---- Compile harvester binaries ----
build: gen-version-env | $(ROOT)/bin
	$(BANNER)
	$(DOCKER_BUILD) --target build-output --output type=local,dest=.


# ---- Validate ----
validate: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target validate


# ---- Test ----
test: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target test -t $(MK_TEST_INTEGRATION_IMAGE)
	docker run $(MK_DOCKER_RUN_OPTS_TTY) --rm --privileged --network host \
	    -v /var/run/docker.sock:/var/run/docker.sock \
		-v /proc:/host/proc \
	    -v seeder-test-go-cache-${MK_REPO_ID}:/go/src/github.com/harvester/seeder/.cache/go-build \
	    $(MK_TEST_INTEGRATION_IMAGE) \
	    ./scripts/test


# ---- Package seeder image ----
package: build
	$(BANNER)
	$(ROOT)/scripts/package

# ---- Generate ----
generate: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target generate-bin-data --output type=local,dest=$(ROOT)/pkg/

# ---- Clean ----
clean:
	$(BANNER)
	@rm -rf $(ROOT)/bin
	@docker rmi -f $(MK_TEST_INTEGRATION_IMAGE) || true

.DEFAULT_GOAL := default

default: build package

ci: validate test build package
