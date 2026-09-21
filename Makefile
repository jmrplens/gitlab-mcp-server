.PHONY: build build-all build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64 \
	run brand brand-check brand-rasters ensure-mcp-publisher mcp-publisher-version test test-short test-race coverage-conditions coverage-mutants test-pkg test-integration test-e2e test-e2e-harness test-e2e-http test-e2e-stdio test-e2e-collector ensure-gotestsum test-e2e-docker test-e2e-docker-enterprise test-e2e-gitlab-com \
	e2e-server-binary test-e2e-ce test-e2e-ee test-e2e-gitlab e2e-clean-orphans \
	validate-http-stateless validate-http-stateless-docker \
	orbit-setup-fixtures orbit-wait-indexer orbit-run-live-tests orbit-ensure-token \
	coverage \
	modeleval-ce modeleval-ee modeleval-probe \
	lint fmt clean version release release-check checksum \
	golangci-lint govulncheck sonar sonar-status \
	mdlint mdlint-fix audit-docs check-doc-links \
	analyze analyze-fix analyze-report install-tools \
	audit-output audit-tokens audit-tools audit-surface-quality check-surface-quality check-spec-conditions audit-metrics audit-dynamic-aliases audit-test-names audit-godocs audit-godocs-check fix-godocs \
	audit-catalog-first \
	audit-struct-completeness audit-action-coverage audit-metadata-completeness audit-1to1 audit-1to1-sdk audit-1to1-enums audit-1to1-paths audit-1to1-paths-endpoints audit-1to1-paths-e2e audit-1to1-validate-docs audit-edition-tier \
	audit-discovery audit-discovery-check audit-e2e-gaps audit-e2e-coverage e2e-go-coverage check-e2e-static audit-gateway-chars check-gateway-chars check-test-file-names audit-test-subtests check-test-subtests check-supply-chain \
	e2e-coverage-record e2e-coverage-record-ce e2e-coverage-record-ee e2e-coverage-record-render check-e2e-coverage-record check-e2e-coverage-page \
	audit-md-escaping check-md-escaping \
	check-em-dash check-pr-description \
	audit-action-ids check-action-ids \
	audit-dead-consts check-dead-consts \
	check-readonly-graphql audit-readonly-graphql \
	audit-meta-descriptions check-meta-descriptions \
	gen-graphql-schema check-graphql-schema check-graphql-documents audit-graphql-documents check-graphql-documents-live check-graphql-shapes audit-graphql-shapes audit-graphql-sent \
	gen-api-live check-api-live check-meta-descriptions \
	record-request-inventory gen-request-inventory check-request-inventory audit-request-inventory \
	audit-doc-coverage audit-doc-coverage-check \
	gen-action-catalog-manifest check-action-catalog-manifest gen-llms check-llms gen-lhm-manifest check-lhm-manifest gen-model-corpus check-model-corpus gen-model-results model-results-record model-results-refold model-results-dry-run check-model-results gen-icon-webp check-icon-webp check-server-json check-server-json-packages check-openplugin audit-doc-tool-names check-doc-tool-names check-install-buttons check-mcpb mcpb gen-npm sync-npm-version validate-npm validate-npm-local publish-npm-dry publish-npm gen-pypi validate-pypi validate-pypi-local publish-pypi-dry publish-pypi gen-nuget validate-nuget validate-nuget-local publish-nuget-dry publish-nuget publish-lobehub gen-readme gen-footprint check-footprint gen-stats check-stats gen-site-stats check-site-stats gen-testing-docs check-testing-docs update-all \
	bench-resources bench-resources-render check-bench-resources bench-fairness \
	docs-local-go \
       docker-build docker-push docker-run \
       inspector inspector-stop help

BINARY_NAME=gitlab-mcp-server
CMD_PATH=./cmd/server
PKGS=./cmd/... ./internal/...
# Keep in lockstep with MCP_PUBLISHER_VERSION in .github/workflows/release.yml:
# this one validates the manifest, that one publishes it. check-server-json
# refuses to run when the two disagree.
MCP_PUBLISHER_VERSION=v1.8.1
# The publisher is installed once per pinned version into a directory named
# after that version, so a stale binary can never validate with another
# release, and CI caches that directory by version instead of resolving the
# module on every push. It is not a go.mod tool on purpose: the registry
# module carries 77 direct dependencies that are not this project's.
MCP_PUBLISHER_DIR=$(HOME)/.cache/gitlab-mcp-server/mcp-publisher/$(MCP_PUBLISHER_VERSION)
MCP_PUBLISHER=$(MCP_PUBLISHER_DIR)/publisher
GO_ANALYSIS_PKGS=./...
# Every e2e build tag, and it must stay every one. A tagged file is invisible
# to go vet and to golangci-lint unless its tag is listed here, so a suite
# added behind a new tag is analysed by nothing until it is added. That has
# happened three times: `httpe2e` was missing until the HTTP suite broke CI,
# `stdioe2e` until the stdio suite did, and `orbitlive` had never been listed
# at all, so test/e2e/orbit had gone unlinted since it was written. The same
# list lives in cmd/gen_testing_docs as e2eTags, for the same reason. Every
# file under test/e2e/gitlab and test/e2e/internal carries
# `e2e` and nothing else, so this one list is the whole of what the analysis
# has to know: no package under test/e2e selects between two file sets any
# more, and one run sees all of them.
GO_ANALYSIS_TAGS=e2e,collectore2e,httpe2e,orbitlive,stdioe2e
PROJECT_GO_VERSION := $(shell awk '/^go / {print $$2; exit}' go.mod)
GO_TOOLCHAIN ?= go$(PROJECT_GO_VERSION)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)

# E2E test report directory (inside dist/, gitignored)
E2E_REPORT_DIR=dist/e2e-reports

# Where the e2e suite records what it asked the server to do and what the
# server dispatched, one shard per test process, under one directory per
# Docker target (dist/e2e-calls/ce, dist/e2e-calls/ee). The shards are a
# byproduct of one run and are never committed; cmd/audit_e2e_coverage reads
# them back. The e2e targets export it as an absolute path, since the recorder
# refuses a relative one for the same reason the request inventory does, and
# clear the target's directory first, so a shard of an earlier run never
# folds into a later one's record. Every one of them runs with -count=1: a
# package-list run can answer from the test cache, and a cached PASS records
# nothing.
E2E_CALLS_DIR=dist/e2e-calls

# COVER=1 measures how much of the server binary an e2e run executed, which is
# a different question from the one the calls directory answers: that one says
# which catalog actions dispatched, and this one says which statements of the
# program ran at all: startup, the catalog build, the middleware chain, the
# error branches no scenario reaches. It is off by default because it costs a
# build of its own and slows every child, and because the merged profile is a
# report rather than a gate: only a live GitLab produces the input, so no
# committed artifact could be verified offline and no floor could be moved
# without booting one.
#
# With it on, the shared build carries -cover and each run target hands its
# children a GOCOVERDIR under here, one directory per target, cleared where the
# calls directory is cleared: in run-docker-e2e.sh for the two Docker targets,
# since CI invokes that script rather than the target, and in the recipe for
# the self-hosted one and for the three transport modules. `make
# e2e-go-coverage` merges what the run left, so a GitLab run and a transport
# run fold into one figure. dist/ is gitignored, so `make clean` removes all of
# it.
#
# Two names for two things, the way E2E_CALLS_DIR and the exported
# GITLAB_MCP_TEST_E2E_CALLS_DIR are two names: E2E_COVER_ROOT is the parent
# every target writes a leaf under and the one `make e2e-go-coverage` searches,
# and E2E_COVER_DIR, exported to the harness and to run-docker-e2e.sh, is the
# one leaf a single run is handed. They were one name until the collision bit:
# CI set it to an absolute leaf for the run and passed it to make as a relative
# parent for the merge, and the two readings only happened to agree.
#
# E2E_COVER_ROOT is repository-relative and must stay so, since the line below
# joins $(CURDIR) onto it: an absolute override would be appended to the
# repository path rather than replacing it, and the counters would land under
# dist/ while the merge looked where the override said. E2E_CALLS_DIR is joined
# the same way, so this is the convention here rather than a property of
# coverage.
COVER ?=
E2E_COVER_ROOT=dist/e2e-cover
E2E_COVER_PROFILE=$(E2E_REPORT_DIR)/e2e-go-coverage.out
# Empty unless COVER is set, so an ordinary run is unaffected: nothing is built
# with -cover and the children carry no GOCOVERDIR at all. Absent rather than
# empty, because the runtime of a child given an empty one prints "GOCOVERDIR
# not set, no coverage data emitted" on its stderr the moment it starts, which
# would head every child's log for the whole run.
e2e_cover_build = $(if $(COVER),-cover)
e2e_cover_env = $(if $(COVER),E2E_COVER_DIR=$(CURDIR)/$(E2E_COVER_ROOT)/$(1))

# The revision the e2e run records on its run line, so a recorded baseline
# says which tree produced it. Overridable for a run of a tree git cannot see.
E2E_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
export E2E_COMMIT

# Where the unit suite records the requests it issues, one shard per test
# process. A shard is a byproduct of one run and only the merged inventory is
# committed, so this lives in the gitignored build directory. The path handed
# to the recorder is absolute because a test binary runs in its own package
# directory.
REQUEST_INVENTORY_SHARDS=dist/request-inventory

# GitLab.com Orbit live-test fixtures. All overridable on the command line
# or via .env. Defaults are designed for the canonical plens1 namespace.
ORBIT_FIXTURES_NAMESPACE ?= plens1
ORBIT_FIXTURES_GITLAB_URL ?= https://gitlab.com
# How long to wait for the indexer to catch up after provisioning fixtures
# (in seconds, polled every 15s). When this elapses the make target
# proceeds with a warning; the live test assertions are tolerant of
# partial indexing (row_count > 0 rather than strict equality).
ORBIT_FIXTURES_INDEXER_TIMEOUT ?= 600
# When set to "true", additionally mirror gitlab-org/cli for realistic
# cross-entity CI/MR data. Adds ~5 min of mirror time on first run.
ORBIT_FIXTURES_MIRROR ?= false
# The -timeout handed to go test by the licensed Docker run, test-e2e-ee. go
# test applies it to each test binary on its own, so with the run naming two
# packages, common and ee, each package gets the whole budget rather than the
# two sharing it. E2E_GITLAB_TIMEOUT below is the same flag for the CE run.
E2E_DOCKER_ENTERPRISE_TIMEOUT ?= 3600s
# Where the e2e Docker fixture is reached from the machine running the suite.
# Docker itself follows DOCKER_HOST or the active context, so the fixture can
# run on another host: `DOCKER_HOST=ssh://truenas
# E2E_DOCKER_GITLAB_URL=http://192.168.0.40:8929 make test-e2e-docker`. GitLab's
# own idea of its URL (external_url, and the registry's beside it) follows the
# same value, so the web_url fields it answers with are the ones the tests
# reach. Bitbucket is published on loopback by default and has to be bound to
# the LAN (E2E_BITBUCKET_BIND=0.0.0.0) when the fixture is remote.
E2E_DOCKER_GITLAB_URL ?= http://localhost:8929
E2E_DOCKER_REGISTRY_URL ?= $(patsubst %:8929,%:5050,$(E2E_DOCKER_GITLAB_URL))
E2E_DOCKER_BITBUCKET_URL ?= http://localhost:7990
E2E_BITBUCKET_BIND ?= 127.0.0.1
export E2E_GITLAB_EXTERNAL_URL = $(E2E_DOCKER_GITLAB_URL)
export E2E_REGISTRY_EXTERNAL_URL = $(E2E_DOCKER_REGISTRY_URL)
export E2E_BITBUCKET_BIND

# Read version from VERSION file (single source of truth)
VERSION := $(strip $(file < VERSION))
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# OS detection for portable commands
ifeq ($(OS),Windows_NT)
  BINARY_EXT=.exe
  MKDIR_P=if not exist $(subst /,\,$1) mkdir $(subst /,\,$1)
  RM_RF=if exist $(subst /,\,$1) rmdir /s /q $(subst /,\,$1)
  RM_F=if exist $(subst /,\,$1) del /q $(subst /,\,$1)
else
  BINARY_EXT=
  MKDIR_P=mkdir -p $1
  RM_RF=rm -rf $1
  RM_F=rm -f $1
endif

# Analysis output directory
ANALYSIS_DIR=dist/analysis
PKGSITE ?= $(shell command -v pkgsite 2>/dev/null || printf "%s/bin/pkgsite" "$$(go env GOPATH 2>/dev/null)")

version: build
	dist/$(BINARY_NAME)$(BINARY_EXT) --version

build:
	go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)$(BINARY_EXT) $(CMD_PATH)

build-all: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64

build-linux-amd64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)

build-linux-arm64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

build-windows-amd64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)

build-windows-arm64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)

build-darwin-amd64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)

build-darwin-arm64:
	$(call MKDIR_P,dist)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

run:
	go run $(CMD_PATH)

## test: run all unit tests with verbose output and coverage profile
test:
	go test -v -coverprofile=coverage.out $(PKGS)

## test-short: run all unit tests (fast, no verbose, no coverage)
test-short:
	go test -count=1 $(PKGS)

## test-race: run all unit tests with race detector enabled
# The timeout is explicit because go test's default is ten minutes per package
# binary and internal/tools alone takes about 976 s under the detector, against
# 113 s without it, so this target could never finish and reported a timeout
# rather than a race (issue 536). 60m is the same bound the Race Detector
# workflow carries, and it is a bound rather than an expectation: it exists to
# end a deadlock with every goroutine stack printed.
RACE_TIMEOUT ?= 60m
test-race:
	go test -v -race -timeout $(RACE_TIMEOUT) -coverprofile=coverage.out $(PKGS)

## test-pkg: run tests for a specific package domain (usage: make test-pkg PKG=branches)
test-pkg:
	go test -v -count=1 ./internal/tools/$(PKG)/

## test-integration: run integration tests (build tag: integration)
test-integration:
	go test -v -tags integration -coverprofile=coverage.out $(PKGS)

## test-e2e: run end-to-end tests against a real GitLab instance (reads GITLAB_URL, GITLAB_TOKEN from .env); an alias of test-e2e-gitlab.
# The self-hosted run under its shortest name. It used to run the suite the
# rebuilt one supersedes, and it points at the rebuilt one now so the name a
# developer already types runs the suite that gates releases.
test-e2e: test-e2e-gitlab

# ensure-gotestsum installs gotestsum on demand, so the e2e targets work on
# a fresh checkout without a separate install-tools step. The tool was an
# unstated assumption of the developer machine before this — the first run
# on a clean Linux box provisioned an entire GitLab stack and then died on
# "gotestsum: command not found" after several minutes.
# GOTESTSUM resolves to the PATH binary when present, else to the location
# go install writes to (GOBIN, falling back to GOPATH/bin). Recursive (=) on
# purpose: the fallback must be re-evaluated after ensure-gotestsum runs,
# because a prerequisite shell cannot extend the PATH of later recipes.
GOTESTSUM_INSTALL_DIR = $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)
GOTESTSUM = $(or $(shell command -v gotestsum 2>/dev/null),$(GOTESTSUM_INSTALL_DIR)/gotestsum)

ensure-gotestsum:
	@command -v gotestsum >/dev/null 2>&1 || test -x "$(GOTESTSUM_INSTALL_DIR)/gotestsum" || { \
		echo "gotestsum not found; installing with go install..."; \
		go install gotest.tools/gotestsum@latest; \
	}

## test-e2e-harness: run the e2e harness library's own tests (no GitLab needed).
# The harness is the only route from an e2e test to a server, so a defect in it
# would surface as a failure in whichever domain test happened to run first.
# These tests hold it on its own terms: the ledger, the waits, the names, the
# runtime guard's message, the environment a child is given, and one run of the
# real binary against a stub GitLab.
test-e2e-harness: ensure-gotestsum
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c '$(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-harness-junit.xml \
	  -- -tags e2e -count=1 -timeout 600s ./test/e2e/internal/...'

# The three transport modules below stage the server the way the rebuilt
# suite's targets do, through e2e-server-binary and E2E_SERVER_BINARY. Each of
# them builds cmd/server once per test binary when the variable is unset, so a
# developer running all three used to pay for three compiles of the same tree;
# with it set the build happens once here and every module drives that file.
#
# Two properties of it are the modules' own and are worth knowing before
# setting it by hand. A path naming nothing, a directory, or a file that cannot
# be executed is refused rather than built around, so a stale dist/e2e fails the
# run instead of being silently replaced by a build of its own. And a
# `go test -race` run refuses a staged binary outright, because the detector
# reaches only a binary compiled with -race and this target compiles plainly;
# nothing here passes -race, and .github/workflows/race.yml runs the modules
# directly and stages nothing, which is what keeps that refusal theoretical
# there.
#
# COVER=1 measures these three like every other run target: the shared build
# carries -cover and each of them hands its children a GOCOVERDIR under its own
# leaf. They used to stage a plain build instead, because an instrumented
# binary given no GOCOVERDIR prints "warning: GOCOVERDIR not set, no coverage
# data emitted" from the meta-data emit its init runs, before the server writes
# anything, and the stdio module requires every stderr line to parse as JSON
# (test/e2e/stdio/transport_test.go). Each module reads E2E_COVER_DIR itself
# now (test/e2e/{stdio,http,collector}/coverage_test.go), so the key is there
# and the line never appears. What that buys is the only measurement of the
# transport there is: pipes, process lifetime, the handler chain, the listener,
# and every flag a scenario against a GitLab cannot reach.

## test-e2e-stdio: run the stdio transport end-to-end module (no GitLab needed; drives the real binary over pipes).
test-e2e-stdio: ensure-gotestsum e2e-server-binary
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	$(if $(COVER),$(call RM_RF,$(E2E_COVER_ROOT)/stdio))
	bash -o pipefail -c 'E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	  $(call e2e_cover_env,stdio) \
	  $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-stdio-junit.xml \
	  -- -tags stdioe2e -count=1 -timeout 900s ./test/e2e/stdio/'

## test-e2e-http: run the HTTP transport end-to-end module (no GitLab needed; nginx layer skips without Docker).
test-e2e-http: ensure-gotestsum e2e-server-binary
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	$(if $(COVER),$(call RM_RF,$(E2E_COVER_ROOT)/http))
	bash -o pipefail -c 'E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	  $(call e2e_cover_env,http) \
	  $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-http-junit.xml \
	  -- -tags httpe2e -count=1 -timeout 900s ./test/e2e/http/'

## test-e2e-collector: run this server's telemetry into a real OpenTelemetry Collector (Docker required; skips cleanly without it).
# Deliberately absent from the push-triggered CI jobs, which run the httpe2e and
# stdioe2e tags and never this one. Those two need no daemon; this one pulls a
# container image, which belongs with the Docker-mode targets rather than in the
# path every commit waits on.
test-e2e-collector: ensure-gotestsum e2e-server-binary
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	$(if $(COVER),$(call RM_RF,$(E2E_COVER_ROOT)/collector))
	bash -o pipefail -c 'E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	  $(call e2e_cover_env,collector) \
	  $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-collector-junit.xml \
	  -- -tags collectore2e -count=1 -timeout 900s ./test/e2e/collector/'

## validate-http-stateless: smoke-validate stateless streamable HTTP with the compiled binary (reads GITLAB_URL, GITLAB_TOKEN from .env)
validate-http-stateless:
	scripts/validate-http-stateless.sh binary

## validate-http-stateless-docker: smoke-validate stateless streamable HTTP with the Docker image
validate-http-stateless-docker:
	scripts/validate-http-stateless.sh docker

## test-e2e-docker: the ephemeral GitLab CE run under its older name; an alias of test-e2e-ce.
# It used to carry a Docker lifecycle of its own, written out here, that ran
# the suite this one replaced. That lifecycle now lives once in
# test/e2e/scripts/run-docker-e2e.sh and the suite it started is gone,
# so this name points at the rebuilt CE run and keeps working for anything
# that still spells it: the same GitLab CE, the same Bitbucket fixture, the
# same runner.
test-e2e-docker: test-e2e-ce

# The rebuilt suite: three packages under test/e2e/gitlab that drive the real
# binary over stdio. Each declares the runtime it needs, so the two Docker
# targets below name packages rather than build tags, and both run `common`.
# The Docker lifecycle lives in test/e2e/scripts/run-docker-e2e.sh rather than
# here, once, and every run carries -p 1 (capability locks are process-local)
# and -count=1 (a cached PASS records no calls), which the script adds.
#
# The server binary is built once and handed to every package through
# E2E_SERVER_BINARY, so the three test binaries do not each build cmd/server.
E2E_SERVER_BINARY=dist/e2e/$(BINARY_NAME)$(BINARY_EXT)
# Per package binary, like E2E_DOCKER_ENTERPRISE_TIMEOUT above: common and ce
# each get the whole of it.
#
# It was 1800s and that was not a budget, it was the wall the run hit. A CE
# run of common measures ~1800s on a developer machine, so the margin was two
# seconds, and CI, on a slower runner, panicked with "test timed out after
# 30m0s" after 1800.077s. The figure is the same 3600s the licensed run uses:
# common grows with every scenario, and a timeout is meant to stop a hang
# rather than to cap a suite that is doing its work.
E2E_GITLAB_TIMEOUT ?= 3600s

## e2e-server-binary: build the server the rebuilt e2e suite drives, once for every package.
# Instrumented under COVER=1: the binary every documented run drives is the one
# built here, so this is where -cover has to enter. The target is .PHONY, so an
# uninstrumented build left by an earlier run can never be reused.
e2e-server-binary:
	$(call MKDIR_P,dist/e2e)
	go build $(e2e_cover_build) -o $(E2E_SERVER_BINARY) $(CMD_PATH)

## test-e2e-ce: start ephemeral GitLab CE (+ Bitbucket fixture), run the common and ce packages of the rebuilt suite, tear down.
test-e2e-ce: ensure-gotestsum e2e-server-binary
	E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	E2E_REPORT_DIR=$(CURDIR)/$(E2E_REPORT_DIR) \
	GITLAB_MCP_TEST_E2E_CALLS_DIR=$(CURDIR)/$(E2E_CALLS_DIR)/ce \
	$(call e2e_cover_env,ce) \
	GOTESTSUM=$(GOTESTSUM) \
	./test/e2e/scripts/run-docker-e2e.sh ce -- -timeout $(E2E_GITLAB_TIMEOUT) ./test/e2e/gitlab/common/ ./test/e2e/gitlab/ce/

## test-e2e-ee: start ephemeral GitLab EE with the cached license or the activation code, run the common and ee packages of the rebuilt suite, tear down.
test-e2e-ee: ensure-gotestsum e2e-server-binary
	E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	E2E_REPORT_DIR=$(CURDIR)/$(E2E_REPORT_DIR) \
	GITLAB_MCP_TEST_E2E_CALLS_DIR=$(CURDIR)/$(E2E_CALLS_DIR)/ee \
	$(call e2e_cover_env,ee) \
	GOTESTSUM=$(GOTESTSUM) \
	./test/e2e/scripts/run-docker-e2e.sh ee -- -timeout $(E2E_DOCKER_ENTERPRISE_TIMEOUT) ./test/e2e/gitlab/common/ ./test/e2e/gitlab/ee/

## test-e2e-docker-enterprise: the licensed Docker run under its older name; an alias of test-e2e-ee.
# The files it used to run, the Enterprise half of the retired suite behind a
# build tag of its own, were deleted once every one of their tests had a
# successor under test/e2e/gitlab/ee, so the licensed run is the rebuilt
# suite's and this name keeps working for anything that still spells it.
test-e2e-docker-enterprise: test-e2e-ee

## test-e2e-gitlab: run the rebuilt suite against a self-hosted GitLab (reads GITLAB_URL, GITLAB_TOKEN from .env); a package the instance cannot serve skips.
test-e2e-gitlab: ensure-gotestsum e2e-server-binary
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	$(call RM_RF,$(E2E_CALLS_DIR)/self-hosted)
	$(call MKDIR_P,$(E2E_CALLS_DIR)/self-hosted)
	$(if $(COVER),$(call RM_RF,$(E2E_COVER_ROOT)/self-hosted))
	bash -o pipefail -c 'E2E_RUNTIME_MISMATCH=skip E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	  $(call e2e_cover_env,self-hosted) \
	  GITLAB_MCP_TEST_E2E_CALLS_DIR=$(CURDIR)/$(E2E_CALLS_DIR)/self-hosted $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-gitlab-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-gitlab-log.json \
	  -- -tags e2e -p 1 -count=1 -timeout $(E2E_GITLAB_TIMEOUT) ./test/e2e/gitlab/...'

## e2e-clean-orphans: delete what earlier runs left on a self-hosted GitLab (reads GITLAB_URL, GITLAB_TOKEN from .env): every project, group and user named with E2E_SWEEP_PREFIX (default e2e-).
# Run by hand and by nothing else: a run sweeps only what carries its own run
# ID, and this is the prefix-wide sweep for the leftovers of a run that could
# not clean up. It is a test of the fixture package because that library is
# importable only from test/e2e, and it skips unless the prefix is set.
E2E_SWEEP_PREFIX ?= e2e-
e2e-clean-orphans:
	bash -o pipefail -c 'if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	  E2E_SWEEP_PREFIX=$(E2E_SWEEP_PREFIX) go test -tags e2e -count=1 -v -run "^TestSweepPrefix_Orphans_OnDemand$$" ./test/e2e/internal/fixture/'

## test-e2e-gitlab-com: end-to-end live test of the Orbit knowledge graph
## handlers against https://gitlab.com. Reads GITLAB_COM_TOKEN from .env,
## provisions the kg-fixtures and security-fixtures projects in
## $(ORBIT_FIXTURES_NAMESPACE) (default: plens1), polls the indexer
## until it has caught up, and runs the orbitlive-tagged live tests.
## Idempotent: the setup script skips already-existing resources, so
## re-running is safe.
##
## Overridable variables (command line or .env):
##   ORBIT_FIXTURES_NAMESPACE      default: plens1
##   ORBIT_FIXTURES_GITLAB_URL     default: https://gitlab.com
##   ORBIT_FIXTURES_INDEXER_TIMEOUT  default: 240s
##   ORBIT_FIXTURES_MIRROR         default: false  (set to "true" to
##                                                  mirror gitlab-org/cli
##                                                  as plens1/glab-mirror)
##
## Examples:
##   make test-e2e-gitlab-com                                # default plens1
##   make test-e2e-gitlab-com ORBIT_FIXTURES_NAMESPACE=acme
##   make test-e2e-gitlab-com ORBIT_FIXTURES_MIRROR=true     # also mirror
test-e2e-gitlab-com: orbit-ensure-token orbit-setup-fixtures orbit-wait-indexer orbit-run-live-tests check-graphql-documents-live
	@echo ""
	@echo "=== test-e2e-gitlab-com complete ==="
	@echo "Reports and timings printed above. To re-run later without"
	@echo "re-provisioning fixtures, just call: make orbit-run-live-tests"

## orbit-ensure-token: validate that .env exists and exports GITLAB_COM_TOKEN.
## Note: Make runs each @-prefixed recipe line in a fresh subshell,
## so . ./.env and the GITLAB_COM_TOKEN check must share a shell
## invocation (joined with \ and wrapped in {}).
orbit-ensure-token:
	@if [ ! -f .env ]; then \
		echo "" 1>&2; \
		echo "ERROR: .env not found in repo root" 1>&2; \
		echo "  cp .env.example .env  # then add: GITLAB_COM_TOKEN=glpat-..." 1>&2; \
		echo "" 1>&2; \
		exit 1; \
	fi
	@. ./.env && { \
		if [ -z "$$GITLAB_COM_TOKEN" ]; then \
			echo "ERROR: GITLAB_COM_TOKEN is not set in .env" 1>&2; \
			exit 1; \
		fi; \
		printf "✓ GITLAB_COM_TOKEN is set (length=%d)\n" "$${#GITLAB_COM_TOKEN}"; \
	}

## orbit-setup-fixtures: idempotently provision the fixture projects.
## Passes the resolved namespace and optional --mirror-cli through to
## the script. Pre-existing resources are detected and skipped.
orbit-setup-fixtures: orbit-ensure-token
	@. ./.env && { \
		mirror_flag=""; \
		if [ "$(ORBIT_FIXTURES_MIRROR)" = "true" ]; then mirror_flag="--mirror-cli"; fi; \
		echo ""; \
		echo "=== Provisioning Orbit fixtures in $(ORBIT_FIXTURES_NAMESPACE) on $(ORBIT_FIXTURES_GITLAB_URL) ==="; \
		export GITLAB_COM_TOKEN \
			ORBIT_FIXTURES_NAMESPACE=$(ORBIT_FIXTURES_NAMESPACE) \
			ORBIT_FIXTURES_GITLAB_URL=$(ORBIT_FIXTURES_GITLAB_URL); \
		./scripts/setup-orbit-fixtures.sh $$mirror_flag; \
	}

## orbit-wait-indexer: poll the Orbit indexer until the projects.indexed
## count reflects our newly-provisioned projects (baseline + 2), or
## until ORBIT_FIXTURES_INDEXER_TIMEOUT elapses. Proceeds with a
## warning in the timeout case — the live test is tolerant of partial
## indexing.
## orbit-wait-indexer: poll the Orbit indexer until the projects.indexed
## count reflects our newly-provisioned projects (baseline + 2), or
## until ORBIT_FIXTURES_INDEXER_TIMEOUT elapses. Proceeds with a
## warning in the timeout case — the live test is tolerant of partial
## indexing.
##
## To stay idempotent, the baseline is taken from the FIRST poll in the
## wait loop, not from a separate pre-flight curl. This keeps the
## target = baseline + 2 invariant valid even when the fixtures have
## already been indexed on a prior run.
orbit-wait-indexer: orbit-ensure-token
	@. ./.env && { \
		echo ""; \
		echo "=== Waiting for Orbit indexer to catch up (timeout: $(ORBIT_FIXTURES_INDEXER_TIMEOUT)s) ==="; \
		attempts=$$(( $(ORBIT_FIXTURES_INDEXER_TIMEOUT) / 15 )); \
		baseline=""; \
		target=""; \
		for i in $$(seq 1 $$attempts); do \
			current=$$(curl -sS "$(ORBIT_FIXTURES_GITLAB_URL)/api/v4/orbit/graph_status?full_path=$(ORBIT_FIXTURES_NAMESPACE)" -H "PRIVATE-TOKEN: $$GITLAB_COM_TOKEN" 2>/dev/null \
				| python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('projects',{}).get('indexed',0))" 2>/dev/null || echo 0); \
			state=$$(curl -sS "$(ORBIT_FIXTURES_GITLAB_URL)/api/v4/orbit/graph_status?full_path=$(ORBIT_FIXTURES_NAMESPACE)" -H "PRIVATE-TOKEN: $$GITLAB_COM_TOKEN" 2>/dev/null \
				| python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('indexing',{}).get('state','?'))" 2>/dev/null || echo "?"); \
			if [ -z "$$baseline" ]; then \
				baseline=$$current; \
				target=$$((baseline + 2)); \
				printf "  baseline: %s projects indexed\n" "$$baseline"; \
				printf "  target:   %s projects (baseline + 2 new fixtures)\n" "$$target"; \
			fi; \
			printf "  [%d/%d] indexed=%s state=%s target=%s\n" "$$i" "$$attempts" "$$current" "$$state" "$$target"; \
			if [ "$$current" -ge "$$target" ] 2>/dev/null; then \
				printf "  ✓ indexer caught up (%s >= %s)\n" "$$current" "$$target"; \
				exit 0; \
			fi; \
			sleep 15; \
		done; \
		echo "  ⚠ indexer did not catch up within $(ORBIT_FIXTURES_INDEXER_TIMEOUT)s" 1>&2; \
		echo "    Proceeding anyway — live test assertions are tolerant of" 1>&2; \
		echo "    partial indexing (row_count > 0 rather than strict equality)" 1>&2; \
	}

## orbit-run-live-tests: run the orbitlive-tagged live tests against
## gitlab.com. Standalone — useful for re-running the tests after
## manual setup or after the indexer has had more time to settle.
##
## The live suite lives at test/e2e/orbit/live_test.go (external
## `orbit_test` package, build tag `orbitlive`), not in
## internal/tools/orbit/ — pointing go test at the unit-test package
## would run only the mock-based tests and silently skip the live
## integration coverage.
orbit-run-live-tests: orbit-ensure-token
	@. ./.env && { \
		echo ""; \
		echo "=== Running live tests (build tag: orbitlive) ==="; \
		export GITLAB_COM_TOKEN \
			ORBIT_FIXTURES_NAMESPACE=$(ORBIT_FIXTURES_NAMESPACE); \
		go test -tags orbitlive -count=1 -v -timeout 300s ./test/e2e/orbit/; \
	}

# A prompt that hands a case its own answer is refused by the unit suite now
# rather than by a target nobody remembers to run: TestContract_NoStimulusNamesItsOwnAnswer
# in internal/testutil/modelcorpus renders every case's stimulus and fails on
# one that names that case's own tool, action or parameter, and
# TestContract_FindsAPlantedAnswer proves the rule can fail. Both run under
# `go test ./internal/...`, so there is nothing to schedule and nothing to
# forget.

# The rebuilt model evaluation: the corpus put to a model against the real
# binary and a real GitLab, recorded as observation and scored afterwards.
#
# The two targets below are the e2e Docker lifecycle with one package swapped
# in, so a model run boots the same GitLab, the same runner and the same
# fixture service the end-to-end suite does. Nothing about them is scheduled:
# a run with a real model costs money at a provider and refuses to start
# without MODELEVAL_SPEND=yes, and a run with no MODELEVAL_MODELS at all skips.
#
# The two record directories are deliberately both under dist/modeleval and
# neither under $(E2E_CALLS_DIR). The coverage audit reads a calls directory's
# parent when it is given one, so a model run recording beside the end-to-end
# suite's shards would fold into the coverage record, and a model call is
# credited to no action by design.
MODELEVAL_DIR=dist/modeleval

## modeleval-ce: put the corpus to the configured models against an ephemeral GitLab CE (usage: MODELEVAL_MODELS=fake:perfect make modeleval-ce)
modeleval-ce: ensure-gotestsum e2e-server-binary
	$(call RM_RF,$(MODELEVAL_DIR)/ce)
	$(call MKDIR_P,$(MODELEVAL_DIR)/ce)
	E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	E2E_REPORT_DIR=$(CURDIR)/$(E2E_REPORT_DIR) \
	E2E_REPORT_NAME=modeleval-ce \
	GITLAB_MCP_TEST_MODELEVAL_DIR=$(CURDIR)/$(MODELEVAL_DIR)/ce \
	GITLAB_MCP_TEST_E2E_CALLS_DIR=$(CURDIR)/$(MODELEVAL_DIR)/ce/e2e-calls \
	GOTESTSUM=$(GOTESTSUM) \
	./test/e2e/scripts/run-docker-e2e.sh ce -- -timeout $(E2E_GITLAB_TIMEOUT) ./test/e2e/modeleval/

## modeleval-ee: the same against an ephemeral licensed GitLab EE, which is what the licensed cases need.
modeleval-ee: ensure-gotestsum e2e-server-binary
	$(call RM_RF,$(MODELEVAL_DIR)/ee)
	$(call MKDIR_P,$(MODELEVAL_DIR)/ee)
	E2E_SERVER_BINARY=$(CURDIR)/$(E2E_SERVER_BINARY) \
	E2E_REPORT_DIR=$(CURDIR)/$(E2E_REPORT_DIR) \
	E2E_REPORT_NAME=modeleval-ee \
	GITLAB_MCP_TEST_MODELEVAL_DIR=$(CURDIR)/$(MODELEVAL_DIR)/ee \
	GITLAB_MCP_TEST_E2E_CALLS_DIR=$(CURDIR)/$(MODELEVAL_DIR)/ee/e2e-calls \
	GOTESTSUM=$(GOTESTSUM) \
	./test/e2e/scripts/run-docker-e2e.sh ee -- -timeout $(E2E_DOCKER_ENTERPRISE_TIMEOUT) ./test/e2e/modeleval/

## modeleval-probe: ask each configured provider whether it accepts the request this repository builds (PAID; needs MODELEVAL_PROBE=yes).
# It needs no GitLab: it never opens an Env, so the bootstrap that probes an
# instance never runs. It is the only paid thing here that is not a run, and it
# is two requests per model plus one slice request, which is a contract check.
modeleval-probe:
	MODELEVAL_PROBE=$${MODELEVAL_PROBE:-yes} \
	go test -tags e2e -count=1 -v -run TestProviderContract ./test/e2e/modeleval/

## coverage-conditions: report the boolean conditions of PKG never evaluated both ways (gobco). A line reported is a missing test case; `&&`, `||` and `!` operands count separately.
coverage-conditions:
	@test -n "$(PKG)" || { echo "usage: make coverage-conditions PKG=./cmd/gen_stats"; exit 2; }
	cd $(PKG) && go run github.com/rillig/gobco@v1.3.4

## coverage-mutants: mutation-test PKG with gremlins, INVERT_LOGICAL on so `&&`/`||` independence is checked. The gate on a changed package is Lived 0 and Not covered 0.
# The recipe itself is scripts/coverage-mutants.sh. It moved out of this file
# when it gained the staging step that makes a `package main` measurable at all
# (issue 872): that needs a copy of the package inside the module, a run against
# the copy and a removal that happens whatever the outcome, and a cleanup
# guaranteed by a trap is not something a one-line make recipe can express. The
# reasons below still describe what it does, and the reason for the staging is
# written beside it.
#
# The per-mutant timeout is derived from PKG's own baseline, because gremlins
# computes it as that baseline times a coefficient and applies no floor. On a
# fast package that product is smaller than the fixed cost of starting `go
# test` at all, so every mutant is reported TIMED OUT having never run: measured
# here, internal/tools/surfaces (0.015s of tests) reported 1 killed and 12 timed
# out, and the same package under a budget that clears the startup cost reports
# 13 killed and none timed out. A timeout is not a kill and gremlins leaves it
# out of the efficacy quotient, so the default flatters exactly the packages it
# never managed to test, and internal/edition reported 0.00% efficacy over four
# mutants none of which ever ran.
#
# MUTANT_BUDGET is the floor in seconds; the coefficient is whatever reaches it,
# never below 8 so a slow package still gets a real multiple of its own runtime.
# A timeout that survives a budget this size is a finding rather than a setting:
# it is a mutant that made the package pathologically slow, which is what
# mutating a memo does, and the answer is a test that asserts the memo.
#
# A package that does not pass its own tests is refused rather than measured.
# The pipeline that reads the baseline duration takes its status from the last
# command in it, so a failing `go test` did not stop the recipe; and a failing
# run still ends in "FAIL <pkg> 1.234s", which the duration pattern matches, so
# the baseline was not even empty. Gremlins would then run against a suite that
# already fails, where every mutant is reported KILLED: a perfect score over a
# broken package, which is the one reading this recipe exists to prevent.
#
# MUTANT_BUDGET is a knob and MUTANT_BUDGET_FLOOR is what it may not go under.
# Raising the budget is the caller's business; lowering it past a few seconds
# recreates the defect this whole recipe exists to prevent, since the budget
# would again be smaller than the cost of starting `go test` and every mutant
# would be reported TIMED OUT having never run. The floor is applied out loud
# rather than silently: a run told to use one second and given ten should say
# so, or the printed budget is a second lie on top of the first.
#
# -count=1 is what makes the coefficient mean what the line above says, and it
# is a second defect rather than the same one. The coefficient is not applied to
# the baseline measured here: gremlins multiplies it by the elapsed time of ITS
# OWN coverage run, and that run is a plain `go test -cover -coverprofile`,
# which Go's test cache answers instantly for a package whose files have not
# changed since it last ran. So the second invocation on an unchanged tree, or
# the first after a `--dry-run` that gathered coverage the same way, measures a
# fraction of a second and derives a per-mutant budget from that fraction.
# Measured here, ./cmd/server (44s of tests) had its coverage served from cache
# in 0.65s and reported 0 killed with every mutant TIMED OUT, which reads
# exactly like a package nothing tests. `go build` ignores a flag it does not
# know, so the same GOFLAGS is safe for the compile gremlins runs around each
# mutant.
MUTANT_BUDGET ?= 30
MUTANT_BUDGET_FLOOR ?= 10
coverage-mutants:
	@test -n "$(PKG)" || { echo "usage: make coverage-mutants PKG=./cmd/gen_stats"; exit 2; }
	@scripts/coverage-mutants.sh $(PKG) $(MUTANT_BUDGET) $(MUTANT_BUDGET_FLOOR)

## coverage: run tests and generate HTML coverage report
coverage: test
	go tool cover -html=coverage.out -o coverage.html

# ─── Static Analysis (individual) ────────────────────────────────────────────
# Documentation URLs for each tool:
#   golangci-lint  https://golangci-lint.run/
#   govulncheck    https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
#   markdownlint   https://github.com/DavidAnson/markdownlint-cli2
# See docs/development/static-analysis.md for full documentation.

## golangci-lint: run configured Go formatters and linters via .golangci.yml.
## Includes govet, modernize, gosec, staticcheck, goimports, gofumpt, and gci.
## Docs: https://golangci-lint.run/
golangci-lint:
	@echo === golangci-lint config verify ===
	golangci-lint config verify
	@echo === golangci-lint fmt ===
	golangci-lint fmt --diff
	@echo === golangci-lint run ===
	golangci-lint run --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS)

## govulncheck: scan Go dependencies for known CVEs using call-graph analysis.
## Only reports vulnerabilities where the vulnerable function is actually called.
## Docs: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
govulncheck:
	./scripts/govulncheck.sh -tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS)

## sonar: run the full SonarCloud pipeline like CI — unit tests with coverage,
## upload via sonar-scanner, poll the Compute Engine task, then print the quality
## gate and key measures. Reads SONARQUBE_TOKEN from .env; analyzes the current
## git branch (override with SONAR_BRANCH=<name>). Exits non-zero if the gate fails.
sonar:
	@./scripts/sonar-scan.sh

## sonar-status: fetch and print the latest SonarCloud quality gate for the
## current branch without re-running tests or re-uploading (SONAR_BRANCH overrides).
sonar-status:
	@./scripts/sonar-scan.sh --no-scan

## mdlint: lint Markdown files for style, consistency, and correctness.
## Excludes plan/ directory (working drafts). Uses .markdownlint-cli2.jsonc.
## Docs: https://github.com/DavidAnson/markdownlint-cli2
mdlint:
	@echo === markdownlint ===
	npx markdownlint-cli2 "**/*.md" "#plan"

## mdlint-fix: auto-fix Markdown lint issues (writes files)
mdlint-fix:
	@echo === markdownlint --fix ===
	npx markdownlint-cli2 --fix "**/*.md" "#plan"

## check-doc-links: verify tracked Markdown/MDX local links resolve, anchor included.
check-doc-links:
	@echo === documentation local links ===
	node scripts/check-doc-links.mjs

## audit-docs: run the complete documentation quality gate.
audit-docs:
	npx markdownlint-cli2 README.md AGENTS.md CLAUDE.md CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md "docs/**/*.md" "test/e2e/**/*.md" "site/src/content/docs/**/*.mdx" "site/src/content/i18n/**/*.md"
	go run ./cmd/format_md_tables/ --check
	go run ./cmd/gen_llms/ --check
	go run ./cmd/gen_lhm_manifest/ --check
	$(MAKE) check-testing-docs
	go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json -check
	$(MAKE) check-doc-links
	go run ./cmd/godoc_tool/ audit
	go run ./cmd/audit_surface_quality/ -view=all
	go run ./cmd/audit_dynamic_aliases/
	cd site && pnpm run check
	cd site && pnpm run build
	cd site && pnpm run lint

# ─── Static Analysis (combined) ─────────────────────────────────────────────
# These targets orchestrate multiple tools for convenience.

## lint: quick lint alias for the configured Go lint/format gate.
lint:
	$(MAKE) golangci-lint

## analyze: run the complete static analysis suite sequentially.
## Use this for full project health check before committing.
## Runs every tool and exits non-zero if any tool fails.
analyze:
	@analysis_status=0; \
	run_check() { \
		step="$$1"; \
		shift; \
		echo "$$step"; \
		output="$$( "$$@" 2>&1 )"; \
		status="$$?"; \
		if [ "$$status" -ne 0 ]; then \
			if [ -n "$$output" ]; then \
				echo "$$output"; \
			fi; \
			echo "FAIL (exit $$status)"; \
			analysis_status=1; \
		else \
			echo "OK"; \
		fi; \
		echo ""; \
	}; \
	echo "============================================================"; \
	echo " Static Analysis Suite - gitlab-mcp-server"; \
	echo "============================================================"; \
	echo "Go toolchain: $$GOTOOLCHAIN (go.mod: $(PROJECT_GO_VERSION))"; \
	echo "Go analysis packages: $(GO_ANALYSIS_PKGS)"; \
	echo "Go analysis build tags: $(GO_ANALYSIS_TAGS)"; \
	echo ""; \
	run_check "[1/21] golangci-lint config verify" golangci-lint config verify; \
	run_check "[2/21] golangci-lint fmt" golangci-lint fmt --diff; \
	run_check "[3/21] golangci-lint run" golangci-lint run --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[4/21] constants nothing reads" go run ./cmd/audit_dead_consts/ -check; \
	run_check "[5/21] govulncheck" ./scripts/govulncheck.sh -tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[6/21] markdownlint" npx markdownlint-cli2 "**/*.md" "#plan"; \
	run_check "[7/21] test-goroutine aborts" go run ./cmd/audit_test_goroutines --check; \
	run_check "[8/21] case loops without subtests" go run ./cmd/audit_test_subtests --check; \
	run_check "[9/21] supply-chain policy" go run ./cmd/audit_supply_chain; \
	run_check "[10/21] Markdown escaping" go run ./cmd/audit_md_escaping --check -fail-unresolved-in internal/toolutil; \
	run_check "[11/21] published action IDs" go run ./cmd/audit_action_ids/ -check -json ''; \
	run_check "[12/21] pinned GraphQL schema" go run ./cmd/gen_graphql_schema/ --check; \
	run_check "[13/21] GraphQL documents" go run ./cmd/audit_graphql_documents/; \
	run_check "[14/21] request paths (R-PATH)" go run ./cmd/audit_1to1/ -scope=paths -gaps-only; \
	run_check "[15/21] meta descriptions" go run ./cmd/audit_meta_descriptions/ -check; \
	run_check "[16/21] pinned live GitLab record" go run ./cmd/gen_api_live/ -check; \
	run_check "[17/21] GraphQL response shapes" go run ./cmd/audit_graphql_shapes/; \
	run_check "[18/21] catalog-first invariants" go run ./cmd/audit_catalog_first/; \
	run_check "[19/21] e2e coverage (static)" go run ./cmd/audit_e2e_coverage/ -static; \
	run_check "[20/21] e2e coverage record" go run ./cmd/audit_e2e_coverage/ -check-record -check-record-page; \
	run_check "[21/21] MCP tool surface quality" go run ./cmd/audit_surface_quality/ -check; \
	echo "============================================================"; \
	if [ "$$analysis_status" -ne 0 ]; then \
		echo "Analysis failed. Review findings above."; \
		echo "============================================================"; \
		exit "$$analysis_status"; \
	fi; \
	echo "Analysis complete. All tools passed."; \
	echo "============================================================"

## analyze-fix: apply automatic fixes from format + lint tools.
## Order: golangci-lint formatters → golangci-lint fixes → markdownlint.
## Always run 'make analyze' after to verify remaining findings.
analyze-fix:
	@echo === Applying automatic fixes ===
	@echo [1/3] golangci-lint fmt
	golangci-lint fmt
	@echo [2/3] golangci-lint run --fix
	-golangci-lint run --fix --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS)
	@echo [3/3] markdownlint --fix
	-npx markdownlint-cli2 --fix "**/*.md" "#plan"
	@echo === Fixes applied. Run 'make analyze' to verify. ===

## analyze-report: generate combined analysis report for LLM consumption.
## Output: dist/analysis/report.txt (Markdown-formatted, one section per tool).
analyze-report:
	$(call MKDIR_P,$(ANALYSIS_DIR))
	@echo "Generating analysis report to $(ANALYSIS_DIR)/report.txt ..."
	@echo "# Static Analysis Report - gitlab-mcp-server" > $(ANALYSIS_DIR)/report.txt
	@echo "# Tools: golangci-lint, govulncheck, markdownlint" >> $(ANALYSIS_DIR)/report.txt
	@echo "# Go analysis packages: $(GO_ANALYSIS_PKGS)" >> $(ANALYSIS_DIR)/report.txt
	@echo "# Go analysis build tags: $(GO_ANALYSIS_TAGS)" >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 1. golangci-lint config verify" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-golangci-lint config verify >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 2. golangci-lint fmt" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-golangci-lint fmt --diff >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 3. golangci-lint run" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-golangci-lint run --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS) >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 4. govulncheck" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-govulncheck -tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS) >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 5. markdownlint" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-npx markdownlint-cli2 "**/*.md" "#plan" >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "Report saved to $(ANALYSIS_DIR)/report.txt"

# ─── Tool Installation ───────────────────────────────────────────────────────
# All tools install into $GOBIN (usually $GOPATH/bin).
# Ensure $GOBIN is in your PATH. See docs/development/static-analysis.md.

## install-tools: install all Go static analysis tools to $GOBIN
install-tools:
	@echo Installing static analysis tools...
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install gotest.tools/gotestsum@latest
	@echo All tools installed.

# ─── Docker ──────────────────────────────────────────────────────────────────

## docker-build: build Docker image tagged with version and latest
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(BINARY_NAME):$(VERSION) \
		-t $(BINARY_NAME):latest \
		.

## docker-push: build and push Docker image to DOCKER_REGISTRY
## Usage: make docker-push DOCKER_REGISTRY=registry.example.com/group/project
docker-push:
ifndef DOCKER_REGISTRY
	$(error DOCKER_REGISTRY is required. Usage: make docker-push DOCKER_REGISTRY=registry.example.com/group/project)
endif
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(DOCKER_REGISTRY):$(VERSION) \
		-t $(DOCKER_REGISTRY):latest \
		.
	docker push $(DOCKER_REGISTRY):$(VERSION)
	docker push $(DOCKER_REGISTRY):latest

GITLAB_URL ?= https://gitlab.com

## docker-run: run the Docker image locally in HTTP mode (port 8080)
## Usage: make docker-run [GITLAB_URL=https://gitlab.example.com]
docker-run:
	docker run --rm -p 8080:8080 \
		$(BINARY_NAME):latest \
		--http \
		--http-addr=0.0.0.0:8080 \
		--gitlab-url="$(GITLAB_URL)"

# ─── LLM Discovery Files ─────────────────────────────────────────────────────

## gen-llms: generate llms.txt and llms-full.txt from registered tools/resources/prompts.
gen-llms:
	go run ./cmd/gen_llms/

## check-llms: validate llms.txt/llms-full.txt are current and structurally valid.
check-llms:
	go run ./cmd/gen_llms/ --check

## gen-model-corpus: regenerate the model evaluation corpus breadth ledger.
gen-model-corpus:
	go run ./cmd/gen_model_corpus/

## check-model-corpus: verify the corpus breadth ledger matches the corpus and
## the catalog this tree builds.
check-model-corpus:
	go run ./cmd/gen_model_corpus/ -check

## gen-model-results: redraw the eight managed result blocks from the committed
## record. It measures nothing and needs no shards, which is why update-all
## runs it: the measurement is a paid run, like the e2e coverage record's half.
gen-model-results:
	go run ./cmd/gen_model_results/ -render

## model-results-record: fold a model evaluation run's shards into the committed
## record and redraw the pages from it (usage: make model-results-record
## MODELEVAL_SHARDS=dist/modeleval/ce). Every row the refusals of the plan's
## section 4.7 name is reported and dropped, the fake provider's included, so a
## run of the fake publishes nothing and says why row by row.
model-results-record:
	$(if $(MODELEVAL_SHARDS),,$(error MODELEVAL_SHARDS is unset: name the run's record directory, e.g. make $@ MODELEVAL_SHARDS=dist/modeleval/ce))
	go run ./cmd/gen_model_results/ -shards $(MODELEVAL_SHARDS) -render

## model-results-refold: re-score a run already published, from the shards it
## left behind (usage: make model-results-refold MODELEVAL_SHARDS=dist/modeleval/ce).
## It drops the rows those shards publish, naming each, and folds them again
## under today's scoring rules and the corpus at HEAD. This is the only path by
## which a corrected rule reaches a row already published: the record holds
## scored columns and a redraw scores nothing, so a run whose shards were not
## kept cannot be re-scored.
model-results-refold:
	$(if $(MODELEVAL_SHARDS),,$(error MODELEVAL_SHARDS is unset: name the run's record directory, e.g. make $@ MODELEVAL_SHARDS=dist/modeleval/ce))
	go run ./cmd/gen_model_results/ -shards $(MODELEVAL_SHARDS) -refold -render

## model-results-dry-run: rehearse the whole publishing path into dist/, without
## touching the committed record or the pages (usage: make model-results-dry-run
## MODELEVAL_SHARDS=dist/modeleval/ce). It is what makes a fake run readable: a
## fold refuses every row the fake would publish, rightly, since the fake answers
## from the corpus's own key, so until this existed the only way to see how a
## result is stored and how a page is drawn from it was to pay for a real run.
## Only that one rule is set aside; a stale, filtered or unobserved run is
## refused here exactly as a real fold would refuse it.
model-results-dry-run:
	$(if $(MODELEVAL_SHARDS),,$(error MODELEVAL_SHARDS is unset: name the run's record directory, e.g. make $@ MODELEVAL_SHARDS=dist/modeleval/ce))
	go run ./cmd/gen_model_results/ -shards $(MODELEVAL_SHARDS) -dry-run -render

## check-model-results: the offline gate over the committed results record and
## the blocks drawn from it. No GitLab, no network and no provider; a tree with
## no record yet passes with a note, which is the state until the first paid run
## is published.
check-model-results:
	go run ./cmd/gen_model_results/ -check

## gen-lhm-manifest: regenerate the tools/prompts/resources arrays in lhm.plugin.json.
gen-lhm-manifest:
	go run ./cmd/gen_lhm_manifest/

## check-lhm-manifest: verify lhm.plugin.json declares the registered MCP surface.
check-lhm-manifest:
	go run ./cmd/gen_lhm_manifest/ --check

# ─── Icon Assets ─────────────────────────────────────────────────────────────

## gen-icon-webp: regenerate the light/dark WebP fallbacks for every icon in
## internal/toolutil/icons.go. Maintainer-only: requires rsvg-convert
## (librsvg) and cwebp (libwebp) on PATH — `brew install librsvg webp` or the
## equivalent apt/dnf packages. Not part of CI; the generated .webp files
## under internal/toolutil/icons/webp/ are committed, so ordinary builds
## never invoke this. Run it after adding or editing an icon.
gen-icon-webp:
	go run ./cmd/gen_icon_webp/

## brand: regenerate every vector brand asset (mark, favicon, in-binary MCP brand mark) from cmd/gen_brand's parametric geometry.
brand:
	go run ./cmd/gen_brand/

## brand-check: verify the committed brand assets match the geometry.
brand-check:
	go run ./cmd/gen_brand/ --check

## brand-rasters: render the committed raster brand assets from the gen_brand vectors.
## Maintainer-only (requires rsvg-convert + cwebp, like gen-icon-webp): banner
## WebP for the README, the OG card, and the marketplace icons.
brand-rasters: brand
	tmp=$$(mktemp) && \
	{ rsvg-convert -w 1280 -h 400 .github/brand/banner.svg -o $$tmp && \
	  cwebp -quiet -lossless $$tmp -o .github/brand/banner.webp; }; \
	status=$$?; rm -f $$tmp; exit $$status
	rsvg-convert -w 1200 -h 630 .github/brand/og.svg -o site/public/og-image.png
	rsvg-convert -w 2560 -h 1280 .github/brand/social.svg -o .github/brand/social.png
	rsvg-convert -w 512 -h 512 site/public/favicon.svg -o mcpb/icon.png
	rsvg-convert -w 400 -h 400 site/public/favicon.svg -o site/public/logo-400.png
	rsvg-convert -w 256 -h 256 site/public/favicon.svg -o site/public/favicon.png

## check-icon-webp: verify the committed WebP icon assets match icons.go.
## Same external-tool requirement as gen-icon-webp.
check-icon-webp:
	go run ./cmd/gen_icon_webp/ --check

## audit-doc-tool-names: report documentation that names a tool no surface
## registers, or a canonical action ID the catalog does not publish. Both
## halves of one sentence: a page teaching a call names the tool on the
## individual surface and the ID on the dynamic one.
audit-doc-tool-names:
	go run ./cmd/audit_doc_tool_names/

## check-doc-tool-names: fail when the documentation names a tool or an action
## ID that does not exist.
check-doc-tool-names:
	go run ./cmd/audit_doc_tool_names/ --check

## check-install-buttons: decode the one-click install payloads and hold them to one configuration.
check-install-buttons:
	go run ./cmd/audit_install_buttons/

## check-server-json: validate server.json with the official MCP Registry publisher.
check-server-json: ensure-mcp-publisher
	@grep -q 'MCP_PUBLISHER_VERSION: "$(MCP_PUBLISHER_VERSION)"' .github/workflows/release.yml || { echo "MCP_PUBLISHER_VERSION $(MCP_PUBLISHER_VERSION) in the Makefile is not the version .github/workflows/release.yml publishes with; keep the two in lockstep"; exit 1; }
	$(MCP_PUBLISHER) validate server.json

## mcp-publisher-version: print the pinned MCP Registry publisher version; CI keys its cache on it.
mcp-publisher-version:
	@echo $(MCP_PUBLISHER_VERSION)

## ensure-mcp-publisher: install the pinned MCP Registry publisher into its version-named directory when it is not there yet.
ensure-mcp-publisher:
	@test -x $(MCP_PUBLISHER) || GOBIN=$(MCP_PUBLISHER_DIR) go install github.com/modelcontextprotocol/registry/cmd/publisher@$(MCP_PUBLISHER_VERSION)

## check-server-json-packages: verify every package server.json declares is
## really published and really installable. Downloads the artifacts, so it needs
## network access; the schema check above cannot see any of this.
check-server-json-packages:
	scripts/validate-server-json-packages.sh

## check-openplugin: validate the Agent Plugins manifests (root plugin.json +
## mcp.json) and the legacy Open Plugins manifest (.plugin/plugin.json).
check-openplugin:
	scripts/check-openplugin.sh

# Pin the MCPB packer CLI for supply-chain integrity (also pinned in scripts/build-mcpb.sh).
MCPB_CLI_VERSION := 2.1.2

## GOLANGCI_LINT_VERSION: the linter release CI installs, read from here by
## .github/workflows/ci.yml so the version lives in one place. Not a go.mod tool
## directive on purpose: golangci-lint advises against building it from source
## (slower, and results can vary with the compiling Go), and its dependency
## tree would land in go.sum for every job that runs `go mod download`. Keep it
## equal to what developers run locally, so `make golangci-lint` means the same
## on both sides of a push.
GOLANGCI_LINT_VERSION := v2.13.1

## check-mcpb: validate the Claude Desktop extension manifest (mcpb/manifest.json).
check-mcpb:
	npx --yes @anthropic-ai/mcpb@$(MCPB_CLI_VERSION) validate mcpb/manifest.json

## mcpb: build the Claude Desktop extension bundle (dist/gitlab-mcp-server.mcpb).
## Cross-compiles the darwin universal binary (lipo) and the windows/amd64 binary,
## then assembles and packs the bundle with scripts/build-mcpb.sh.
mcpb:
	@command -v lipo >/dev/null || { echo "ERROR: lipo is required (macOS Xcode CLT)"; exit 1; }
	@set -e; \
	VER=$$(tr -d '[:space:]' < VERSION); \
	rm -rf dist/local_darwin_arm64 dist/local_darwin_amd64 dist/local_darwin_all dist/local_windows_amd64; \
	mkdir -p dist/local_darwin_arm64 dist/local_darwin_amd64 dist/local_darwin_all dist/local_windows_amd64; \
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=$$VER" -o dist/local_darwin_arm64/gitlab-mcp-server ./cmd/server; \
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$$VER" -o dist/local_darwin_amd64/gitlab-mcp-server ./cmd/server; \
	lipo -create -output dist/local_darwin_all/gitlab-mcp-server dist/local_darwin_arm64/gitlab-mcp-server dist/local_darwin_amd64/gitlab-mcp-server; \
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$$VER" -o dist/local_windows_amd64/gitlab-mcp-server.exe ./cmd/server; \
	bash scripts/build-mcpb.sh "$$VER"

## gen-npm: assemble the npm distribution (launcher + 6 per-platform packages).
## Reads release binaries from a directory of assets named as published
## (gitlab-mcp-server-linux-amd64, ...). Version defaults to VERSION.
##   make gen-npm NPM_BINARIES=dist
gen-npm:
	@command -v node >/dev/null || { echo "ERROR: Node.js is required"; exit 1; }
	@test -n "$(NPM_BINARIES)" || { echo "ERROR: set NPM_BINARIES=<dir of release binaries>"; exit 1; }
	node scripts/build-npm.mjs --binaries "$(NPM_BINARIES)" --version "$$(tr -d '[:space:]' < VERSION)"

## sync-npm-version: rewrite npm/gitlab-mcp-server/package.json version + pins to VERSION.
## Run by the release version-stamp; safe to run anytime, needs no binaries.
sync-npm-version:
	@command -v node >/dev/null || { echo "ERROR: Node.js is required"; exit 1; }
	node scripts/build-npm.mjs --sync-only --version "$$(tr -d '[:space:]' < VERSION)"

## validate-npm: build the npm packages and validate them inside a clean
## node:22 container, so the check never installs anything on the host. Runs
## structural checks on all 7 packages plus a real install + MCP handshake for
## the container's native platform (linux-x64).
##   make validate-npm NPM_BINARIES=dist
validate-npm:
	@command -v docker >/dev/null || { echo "ERROR: Docker is required for isolated validation (or use validate-npm-local)"; exit 1; }
	@test -n "$(NPM_BINARIES)" || { echo "ERROR: set NPM_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); 	docker run --rm 		-v "$(CURDIR):/work" -v "$(abspath $(NPM_BINARIES)):/binaries:ro" 		-w /work node:22 sh -euc '			node scripts/build-npm.mjs --binaries /binaries --version '"$$VER"' && 			node scripts/validate-npm.mjs --packages npm/packages --main npm/gitlab-mcp-server --version '"$$VER"

## validate-npm-local: same validation without Docker, for the ephemeral CI
## runner (already disposable). Builds packages from NPM_BINARIES, then validates.
validate-npm-local:
	@command -v node >/dev/null || { echo "ERROR: Node.js is required"; exit 1; }
	@test -n "$(NPM_BINARIES)" || { echo "ERROR: set NPM_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); 	node scripts/build-npm.mjs --binaries "$(NPM_BINARIES)" --version "$$VER" && 	node scripts/validate-npm.mjs --packages npm/packages --main npm/gitlab-mcp-server --version "$$VER"

## publish-npm-dry: assemble and validate the npm publish set without publishing.
publish-npm-dry:
	@test -n "$(NPM_BINARIES)" || { echo "ERROR: set NPM_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-npm.sh "$(NPM_BINARIES)" "$$(tr -d '[:space:]' < VERSION)" --dry-run

## publish-npm: assemble and publish the npm distribution (platform packages, then launcher).
## Auth via npm login or NPM_TOKEN. See the release process in CLAUDE.md.
publish-npm:
	@test -n "$(NPM_BINARIES)" || { echo "ERROR: set NPM_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-npm.sh "$(NPM_BINARIES)" "$$(tr -d '[:space:]' < VERSION)"

## gen-pypi: assemble the PyPI wheelhouse (six platform wheels) from release
## binaries. The wheels carry the native binary in .data/scripts, the uv/ruff
## model: the installer puts it on the scripts path as the command itself.
##   make gen-pypi PYPI_BINARIES=dist
gen-pypi:
	@command -v python3 >/dev/null || { echo "ERROR: Python 3 is required"; exit 1; }
	@test -n "$(PYPI_BINARIES)" || { echo "ERROR: set PYPI_BINARIES=<dir of release binaries>"; exit 1; }
	python3 scripts/build_pypi.py --binaries "$(PYPI_BINARIES)" --version "$$(tr -d '[:space:]' < VERSION)"

## validate-pypi: build the wheels and validate them inside a clean
## python:3.14-slim container: RECORD hashes, metadata, ownership token,
## binary magic and glibc floor for all six, plus a real venv install and MCP
## initialize handshake for the container's native platform.
##   make validate-pypi PYPI_BINARIES=dist
validate-pypi:
	@command -v docker >/dev/null || { echo "ERROR: Docker is required for isolated validation (or use validate-pypi-local)"; exit 1; }
	@test -n "$(PYPI_BINARIES)" || { echo "ERROR: set PYPI_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); \
	docker run --rm \
		-v "$(CURDIR):/work" -v "$(abspath $(PYPI_BINARIES)):/binaries:ro" \
		-w /work python:3.14-slim sh -euc ' \
			python3 scripts/build_pypi.py --binaries /binaries --version '"$$VER"' && \
			python3 scripts/validate_pypi.py --wheels pypi/dist --version '"$$VER"

## validate-pypi-local: same validation without Docker, for the ephemeral CI
## runner (already disposable). Builds wheels from PYPI_BINARIES, then validates.
validate-pypi-local:
	@command -v python3 >/dev/null || { echo "ERROR: Python 3 is required"; exit 1; }
	@test -n "$(PYPI_BINARIES)" || { echo "ERROR: set PYPI_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); \
	python3 scripts/build_pypi.py --binaries "$(PYPI_BINARIES)" --version "$$VER" && \
	python3 scripts/validate_pypi.py --wheels pypi/dist --version "$$VER"

## publish-pypi-dry: assemble and validate the PyPI wheelhouse without uploading.
publish-pypi-dry:
	@test -n "$(PYPI_BINARIES)" || { echo "ERROR: set PYPI_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-pypi.sh "$(PYPI_BINARIES)" "$$(tr -d '[:space:]' < VERSION)" --dry-run

## publish-pypi: assemble, validate and publish the PyPI wheels out of band.
## Auth via PYPI_TOKEN. The release workflow publishes through the OIDC
## trusted publisher instead; this target is for bootstrap/manual publishes.
publish-pypi:
	@test -n "$(PYPI_BINARIES)" || { echo "ERROR: set PYPI_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-pypi.sh "$(PYPI_BINARIES)" "$$(tr -d '[:space:]' < VERSION)"

## gen-nuget: assemble the NuGet distribution (a pointer package plus six
## runtime-identifier packages) from release binaries. The .NET 10 tool
## layout: the pointer names a package per runtime identifier, each of which
## carries the native binary as the command's entry point. Packs with the
## standard library, so no .NET SDK is needed here.
##   make gen-nuget NUGET_BINARIES=dist
gen-nuget:
	@command -v python3 >/dev/null || { echo "ERROR: Python 3 is required"; exit 1; }
	@test -n "$(NUGET_BINARIES)" || { echo "ERROR: set NUGET_BINARIES=<dir of release binaries>"; exit 1; }
	python3 scripts/build_nuget.py --binaries "$(NUGET_BINARIES)" --version "$$(tr -d '[:space:]' < VERSION)"

## validate-nuget: build the packages and validate them inside a clean .NET
## SDK container: OPC layout, nuspec metadata, tool manifests, ownership
## token, binary magic and executable bit for all six, plus a real
## `dotnet tool install`, a `dnx` run and an MCP initialize handshake for the
## container's native platform. The image is pinned by digest because dnx's
## behaviour is an SDK property (10.0.400 is what was verified); bump the
## digest deliberately.
##   make validate-nuget NUGET_BINARIES=dist
NUGET_SDK_IMAGE=mcr.microsoft.com/dotnet/sdk:10.0@sha256:e1ffd2a92ae84c1291bc1b6887501f8af98e6331e7af6d4c8d37168c5e87a64c
validate-nuget:
	@command -v docker >/dev/null || { echo "ERROR: Docker is required for isolated validation (or use validate-nuget-local)"; exit 1; }
	@test -n "$(NUGET_BINARIES)" || { echo "ERROR: set NUGET_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); \
	docker run --rm \
		-v "$(CURDIR):/work" -v "$(abspath $(NUGET_BINARIES)):/binaries:ro" \
		-e DOTNET_NOLOGO=1 -e DOTNET_CLI_TELEMETRY_OPTOUT=1 \
		-w /work $(NUGET_SDK_IMAGE) sh -euc ' \
			apt-get -qq update && apt-get -qq install -y --no-install-recommends python3 >/dev/null && \
			python3 scripts/build_nuget.py --binaries /binaries --version '"$$VER"' && \
			python3 scripts/validate_nuget.py --packages nuget/dist --version '"$$VER"

## validate-nuget-local: same validation without Docker, for the ephemeral CI
## runner (already disposable; the .NET 10 SDK must be on PATH). Builds the
## packages from NUGET_BINARIES, then validates.
validate-nuget-local:
	@command -v python3 >/dev/null || { echo "ERROR: Python 3 is required"; exit 1; }
	@command -v dotnet >/dev/null || { echo "ERROR: the .NET 10 SDK is required (or use validate-nuget)"; exit 1; }
	@test -n "$(NUGET_BINARIES)" || { echo "ERROR: set NUGET_BINARIES=<dir of release binaries>"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); \
	python3 scripts/build_nuget.py --binaries "$(NUGET_BINARIES)" --version "$$VER" && \
	python3 scripts/validate_nuget.py --packages nuget/dist --version "$$VER"

## publish-nuget-dry: assemble and validate the NuGet packages without pushing.
publish-nuget-dry:
	@test -n "$(NUGET_BINARIES)" || { echo "ERROR: set NUGET_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-nuget.sh "$(NUGET_BINARIES)" "$$(tr -d '[:space:]' < VERSION)" --dry-run

## publish-nuget: assemble, validate and push the NuGet packages out of band.
## Auth via NUGET_API_KEY (an API key from nuget.org). The release workflow
## pushes under the nuget.org trusted publishing policy instead, with a
## one-hour key minted from its OIDC identity; this is the only tokened path.
publish-nuget:
	@test -n "$(NUGET_BINARIES)" || { echo "ERROR: set NUGET_BINARIES=<dir of release binaries>"; exit 1; }
	scripts/publish-nuget.sh "$(NUGET_BINARIES)" "$$(tr -d '[:space:]' < VERSION)"

## publish-lobehub: push the current version of the existing LobeHub listing.
## Reads lhm.plugin.json (version kept in sync by scripts/update-server-json-sha.sh
## on each release) and posts it via the @lobehub/market-cli. The CLI split its
## verbs: `plugin publish <gitUrl>` is for FIRST-TIME listings only and now
## requires the git URL; an already-published plugin is refused with "use lhm
## plugin update", which is what this target runs. Requires a one-time
## interactive `lhm login` + `lhm github connect` on the machine — LobeHub has
## no CI path. See the release process in CLAUDE.md.
# Pinned: the CLI has already changed its verb contract under this target
# once (publish -> update, a new positional gitUrl); bump deliberately after
# verifying `plugin update --dir` still holds.
LOBEHUB_MARKET_CLI_VERSION := 0.0.41

publish-lobehub: check-lhm-manifest
	@command -v node >/dev/null || { echo "ERROR: Node.js >= 22 is required"; exit 1; }
	@NODE_MAJOR=$$(node -v | sed 's/^v\([0-9]*\).*/\1/'); \
	if [ "$$NODE_MAJOR" -lt 22 ]; then echo "ERROR: Node.js >= 22 is required (found $$(node -v))"; exit 1; fi
	@command -v jq >/dev/null || { echo "ERROR: jq is required"; exit 1; }
	@VER=$$(tr -d '[:space:]' < VERSION); \
	MVER=$$(jq -r '.version' lhm.plugin.json); \
	if [ "$$VER" != "$$MVER" ]; then \
		echo "ERROR: VERSION ($$VER) != lhm.plugin.json version ($$MVER); run a release stamp first"; exit 1; \
	fi; \
	echo "Updating jmrplens-gitlab-mcp-server to v$$VER on LobeHub..."; \
	npx -y @lobehub/market-cli@$(LOBEHUB_MARKET_CLI_VERSION) plugin update --dir "$(CURDIR)"

## gen-readme: regenerate all managed README.md sections (token footprint + stats).
gen-readme: gen-footprint gen-stats

## update-all: run every generator, the brand assets included, then the table formatter.
## Generates: brand vectors, token footprint, repo stats, site stats, llms.txt, LobeHub manifest, testing docs, action catalog manifest, benchmark charts and tables, markdown table formatting.
# One generator at a time, in the recipe rather than as prerequisites: brand
# rewrites internal/toolutil/brandmark_gen.go, which the generators after it
# compile, and gen-footprint and gen-stats both rewrite README.md, so make -j
# would interleave them. bench-resources-render is in because it redraws from
# the committed record and measures nothing, which is what check-bench-resources
# then compares; bench-resources itself stays out, and so does brand-rasters,
# which needs rsvg-convert and cwebp that only the maintainer's machine has.
# e2e-coverage-record-render is in on exactly the same terms and
# e2e-coverage-record is out on the same ones: redrawing the coverage page
# needs only the committed record, while measuring it needs a booted GitLab.
# gen-model-results is in and model-results-record is out for the third time on
# those terms, and the measurement there is not merely slow: it is a paid run
# against a provider.
update-all:
	@for target in brand gen-footprint gen-stats gen-site-stats gen-llms gen-lhm-manifest gen-model-corpus gen-model-results gen-testing-docs gen-action-catalog-manifest bench-resources-render e2e-coverage-record-render; do \
		$(MAKE) --no-print-directory $$target || exit 1; \
	done
	go run ./cmd/format_md_tables/
	@echo "All generators and formatters complete."

## gen-footprint: measure token footprint and write the README token-claim block and footprint section, token-footprint.md and site/src/data/token-footprint.json.
gen-footprint:
	go run ./cmd/audit_tokens/ -footprint

## check-footprint: verify the README token-claim block and footprint section, token-footprint.md and site/src/data/token-footprint.json are current.
check-footprint:
	go run ./cmd/audit_tokens/ -footprint -check

## gen-stats: regenerate the repository statistics section in README.md.
gen-stats:
	go run ./cmd/gen_stats/

## check-stats: verify the README repository statistics section is current.
check-stats:
	go run ./cmd/gen_stats/ -check

## gen-site-stats: regenerate the single-sourced site stats JSON (site/src/data/stats.json).
gen-site-stats:
	go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json

## check-site-stats: verify the committed site stats JSON is current.
check-site-stats:
	go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json -check

## bench-resources: measure what the server costs to run and redraw the charts.
## Starts the real binary on both transports against an in-process stand-in
## GitLab, so it needs no instance and no credentials. Takes several minutes:
## every scenario builds a tool catalog per client, which is the cost being
## measured. Writes site/src/data/resource-benchmark.json, the SVG pairs under
## docs/reference/benchmarks and site/public/benchmarks, and the generated
## blocks in the three documentation pages.
bench-resources:
	go run ./cmd/bench_resources/

## bench-resources-render: redraw the charts and tables from the committed
## measurements, without re-measuring. This is what to run after changing a
## figure; the numbers stay exactly as they were published.
bench-resources-render:
	go run ./cmd/bench_resources/ -render

## check-bench-resources: verify the committed charts and tables match the
## committed measurements. Seconds, and no benchmark is run, which is what
## makes it a CI gate.
check-bench-resources:
	go run ./cmd/bench_resources/ -check

## bench-fairness: measure whether a bound leaves the quiet tenant better off.
## Two populations of credentials driven differently against one server, twice:
## with the bound in force and without it. Reports the quiet population's served
## and refused counts separately, and can answer that the bound helped nobody.
## Minutes on a modest host. Writes bench/fairness.json, which is not committed,
## and draws no chart: it touches neither the published record nor the artifacts
## check-bench-resources compares. BOUND selects which limit is measured.
BOUND ?= tools-call-rps
bench-fairness:
	go run ./cmd/bench_resources/ -fairness $(BOUND)

## gen-testing-docs: regenerate testing.md counts and coverage tables.
## Runs unit-test coverage over ./cmd/... and ./internal/..., so it takes minutes.
## Add -skip-coverage to refresh only the counts, keeping the recorded values.
gen-testing-docs:
	go run ./cmd/gen_testing_docs/

## check-testing-docs: verify everything in testing.md that a checkout determines.
## Seconds, not minutes: a check carries the recorded coverage values forward
## instead of recomputing them, because those depend on the machine (privilege,
## and whether rsvg-convert is installed) while counts and package rows do not.
## This is the CI gate. To verify the coverage values too, regenerate with
## gen-testing-docs and look at the diff.
##
## No -skip-coverage here on purpose: --check implies it, so every caller gets
## the cheap answer rather than whoever remembered the flag. The command says
## so, and `--check -skip-coverage=false` is the explicit way to measure.
check-testing-docs:
	go run ./cmd/gen_testing_docs/ --check

## gen-action-catalog-manifest: regenerate the ActionSpec group builder manifest.
gen-action-catalog-manifest:
	go run ./cmd/gen_action_catalog_manifest/

## check-action-catalog-manifest: verify the generated ActionSpec manifest is current.
check-action-catalog-manifest:
	go run ./cmd/gen_action_catalog_manifest/ --check

# ─── Output Quality Audit ────────────────────────────────────────────────────

## audit-surface-quality: consolidated MCP tool surface quality audit (both views).
## Combines the former audit_tools (metadata) and audit_output (output quality).
audit-surface-quality:
	go run ./cmd/audit_surface_quality/

## check-surface-quality: the same audit as a gate. Prints the violations that
## gate and exits non-zero on any. The result-envelope section stays a report.
check-surface-quality:
	@echo === MCP tool surface quality ===
	go run ./cmd/audit_surface_quality/ -check

## check-spec-conditions: condition coverage (gobco) over the action_specs.go
## this branch changes against BASE (default origin/main). A condition no test
## evaluates both ways fails; a package gobco cannot instrument is reported.
check-spec-conditions:
	@echo === action spec conditions ===
	./scripts/check-spec-conditions.sh $(BASE)

## audit-output: run MCP output quality audit on all tools.
## Backward-compat wrapper over audit-surface-quality -view=output.
audit-output:
	go run ./cmd/audit_surface_quality/ -view=output

## audit-tokens: measure LLM context window overhead of all tool definitions.
## Reports per-tool token counts, domain totals, and mode comparison.
## Use --compare-schemas for the meta-tool InputSchema sizing spike.
audit-tokens:
	go run ./cmd/audit_tokens/

## audit-tools: audit MCP tool metadata violations (naming, annotations).
## Backward-compat wrapper over audit-surface-quality -view=metadata.
audit-tools:
	go run ./cmd/audit_surface_quality/ -view=metadata

## audit-metrics: report MCP tool metrics (tool/resource/prompt counts).
audit-metrics:
	go run ./cmd/audit_metrics/

## audit-catalog-first: enforce catalog-first registration invariants (ADR-0004).
audit-catalog-first:
	go run ./cmd/audit_catalog_first/

## audit-action-spec-coverage: backward-compat wrapper for audit-catalog-first.
audit-action-spec-coverage:
	go run ./cmd/audit_catalog_first/

## audit-1to1: run all four 1:1-audit gap streams (struct/action/metadata/enums), merge
## into plan/1to1-backlog.json, then gate on SDK parity (audit-1to1-sdk).
## Single binary cmd/audit_1to1 consolidates the former audit_struct_completeness,
## audit_action_coverage, audit_metadata_completeness, and gen_1to1_backlog.
## The backlog is written first so a gate failure still leaves the artifact behind.
audit-1to1:
	go run ./cmd/audit_1to1/ -gaps-only -output plan/1to1-backlog.json
	@echo "1:1 audit backlog written to plan/1to1-backlog.json"
	$(MAKE) audit-1to1-sdk
	$(MAKE) audit-1to1-paths

## audit-1to1-sdk: gate every client-go service, every raw-GraphQL operation and every
## enum value on a decision (R-SERVICE/R-GRAPHQL/R-ENUM). Unlike the three candidate
## streams this one FAILS on a finding: a new SDK service that is neither called nor
## declared, a raw-GraphQL operation whose wrapper now exists and is not adjudicated,
## an SDK enum constant the surface does not offer (or a value it offers that the SDK
## does not declare), or a declaration or exemption that has gone stale.
audit-1to1-sdk:
	go run ./cmd/audit_1to1/ -scope=sdk -gaps-only

## audit-1to1-paths: gate the request an action actually issues (R-PATH). The other
## five rules describe the surface and none of them looks at the request a handler
## builds, which is how nine registered tools shipped unable to work. This FAILS on a
## GraphQL document the pinned schema refuses, on an action whose owner names no
## package under internal/tools, on a package the catalog owns actions in that neither
## the committed inventory shows issuing a request nor declares a reason, and on a
## declaration that no longer describes the tree. A silent package means one of two
## things and the finding cannot tell them apart: no test drives that package, or the
## inventory is stale (`make gen-request-inventory`). It reads the committed inventory,
## so it needs no network and no suite run.
audit-1to1-paths:
	go run ./cmd/audit_1to1/ -scope=paths -gaps-only

## audit-1to1-paths-endpoints: the same, plus every recorded REST endpoint compared with
## GitLab's own API documentation. Needs the network and reads ~250 pages (cached for a
## week afterwards). It FAILS on an endpoint no declaration in
## cmd/audit_1to1/internal/paths/endpoint_declarations.go accounts for, and on a
## declaration that accounts for nothing any more. The declarations are what make the
## comparison safe to gate on: the oracle is prose, so a deprecated-but-working alias,
## an endpoint documented under doc/user, and one whose only mention is a sentence all
## look exactly like a defect, and each of those is written down with its reason.
audit-1to1-paths-endpoints:
	go run ./cmd/audit_1to1/ -scope=paths -check-endpoints -output plan/1to1-paths.json
	@echo "R-PATH report written to plan/1to1-paths.json"

## audit-1to1-paths-e2e: the same report, plus the observation question asked per ACTION
## instead of per owning package, read from the shards a Docker end-to-end run left under
## $(E2E_CALLS_DIR) (make test-e2e-ce or make test-e2e-ee writes them). The committed
## inventory can only answer at package grain, because nothing on the wire names an
## action; a trace does, so the record says which actions were themselves seen issuing a
## request and which ran without issuing one. It changes nothing the gate fails on: the
## shards are a byproduct of a run CI does not schedule, and the counts are floors, since
## a batch queue that overflowed drops client spans without saying so.
audit-1to1-paths-e2e:
	go run ./cmd/audit_1to1/ -scope=paths -e2e-calls $(E2E_CALLS_DIR) -output plan/1to1-paths-e2e.json
	@echo "R-PATH report with per-action observation written to plan/1to1-paths-e2e.json"

## audit-1to1-enums: the enum value rule on its own (R-ENUM), in its native shape.
## Fails on a value the SDK declares that no schema enum or description offers, on a
## value offered that the SDK does not declare, or on a stale exemption.
audit-1to1-enums:
	go run ./cmd/audit_1to1/ -scope=enums -gaps-only

## audit-1to1-validate-docs: verify every doc/api citation behind the 1:1 adjudication tables is still fetchable.
audit-1to1-validate-docs:
	go run ./cmd/audit_1to1/ -validate-docs

## audit-struct-completeness: diff MCP input/output structs vs client-go fields (R-INPUT/R-OUTPUT).
## Backward-compat wrapper over audit-1to1 -scope=structs.
audit-struct-completeness:
	go run ./cmd/audit_1to1/ -scope=structs -gaps-only

## audit-action-coverage: report client-go SDK endpoints no MCP action invokes (R-ACTION).
## Backward-compat wrapper over audit-1to1 -scope=actions.
audit-action-coverage:
	go run ./cmd/audit_1to1/ -scope=actions -gaps-only

## audit-metadata-completeness: report discovery-metadata gaps across the ActionSpec catalog (R-META).
## Backward-compat wrapper over audit-1to1 -scope=metadata.
audit-metadata-completeness:
	go run ./cmd/audit_1to1/ -scope=metadata -gaps-only

## audit-edition-tier: report each action's doc-grounded licensing tier vs current gating.
audit-edition-tier:
	go run ./cmd/audit_edition_tier/ -gaps-only

## audit-discovery: report discovery-metadata gaps (aliases/usage/related/param-guidance/sibling-cluster) across the ActionSpec catalog (META-001).
audit-discovery:
	go run ./cmd/audit_discovery_completeness/ -gaps-only -output plan/discovery-backlog.json

## audit-discovery-check: CI gate for META-001. Exits non-zero when any error-severity finding is present.
## Note: post-Phase-0 baseline has ~439 errors (real findings); this gate is
## designed to drive Phase 1+ waves to zero. Use `make audit-discovery` for
## the human-readable report.
audit-discovery-check:
	go run ./cmd/audit_discovery_completeness/ -gaps-only -check

## audit-doc-coverage: report per-doc-file gaps between docs/tools/*.md and the canonical action catalog (DOC-002).
## Writes the per-file backlog to plan/docs-tools-backlog.json (gitignored) so each Phase-1 doc-writer can pick a file with full context.
audit-doc-coverage:
	$(call MKDIR_P,plan)
	go run ./cmd/audit_doc_coverage/ -output plan/docs-tools-backlog.json

## audit-doc-coverage-check: CI gate for DOC-002. Exits non-zero when any docs/tools/*.md has missing/orphan/tier_mismatch findings.
## Use `make audit-doc-coverage` for the full human-readable report; use this for pre-PR gating.
audit-doc-coverage-check:
	go run ./cmd/audit_doc_coverage/ -check

## audit-dynamic-aliases: audit Dynamic search aliases and canonical action reachability.
audit-dynamic-aliases:
	go run ./cmd/audit_dynamic_aliases/

## audit-e2e-gaps: report catalog actions the e2e suite does not exercise, as
## a TSV work list. It reads the same recorded calls audit-e2e-coverage does,
## so a gap here is an action no test dispatched rather than an action no
## source file mentions.
audit-e2e-gaps:
	go run ./cmd/audit_e2e_coverage/ -calls $(E2E_CALLS_DIR) -report

## audit-e2e-coverage: report what the e2e suite covered, from the calls it
## recorded rather than from mentions in its source: one runtime per directory
## under $(E2E_CALLS_DIR), every runtime x surface x mode x action classified,
## the levels L1 to L3, and the non-tool capabilities. Needs a run that had
## GITLAB_MCP_TEST_E2E_CALLS_DIR set; the JSON goes to $(E2E_REPORT_DIR).
audit-e2e-coverage:
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	go run ./cmd/audit_e2e_coverage/ -calls $(E2E_CALLS_DIR) -report -o $(E2E_REPORT_DIR)/e2e-coverage.json -summary -

## e2e-coverage-record: fold both Docker runs into the committed per-runtime
## coverage record and redraw the page beside it. Needs the shards and the
## gotestsum streams of a run of each half, so it is a maintainer's command
## and deliberately not a member of update-all. Each half is written by its
## own invocation: one -results stream belongs to one runtime, and joining
## the ce stream to the ee shards would credit each against the other's tests.
e2e-coverage-record:
	@for target in e2e-coverage-record-ce e2e-coverage-record-ee; do \
		$(MAKE) --no-print-directory $$target || exit 1; \
	done

## e2e-coverage-record-ce: record the unlicensed half alone, for a maintainer
## who ran only `make test-e2e-ce`. The write folds into the committed
## document and leaves the ee entry exactly as it was.
e2e-coverage-record-ce:
	go run ./cmd/audit_e2e_coverage/ -calls $(E2E_CALLS_DIR)/ce -results $(E2E_REPORT_DIR)/e2e-ce-log.json \
		-runtime ce -static -record -o $(E2E_REPORT_DIR)/e2e-coverage-ce.json

## e2e-coverage-record-ee: record the licensed half alone, for a maintainer
## who ran only `make test-e2e-ee`.
e2e-coverage-record-ee:
	go run ./cmd/audit_e2e_coverage/ -calls $(E2E_CALLS_DIR)/ee -results $(E2E_REPORT_DIR)/e2e-ee-log.json \
		-runtime ee -static -record -o $(E2E_REPORT_DIR)/e2e-coverage-ee.json

## e2e-coverage-record-render: redraw docs/development/testing/e2e-coverage.md
## from the committed record, without a GitLab. This is what to run after
## changing the page's wording; the figures stay exactly as they were measured.
e2e-coverage-record-render:
	go run ./cmd/audit_e2e_coverage/ -render-record

## check-e2e-coverage-record: the offline gate over the committed coverage
## record: both runtimes present, the levels agreeing with the action lists
## beside them, the floors of -check re-applied, and the record inside its own
## staleness window. No GitLab, no Docker and no network; a catalog that has
## moved under the record is a note rather than a failure, since
## check-e2e-static already fails on a rename from the scenario's side. The
## page beside the record is check-e2e-coverage-page, which is a separate
## target because it is a freshness comparison and this one is not: nothing
## here can regenerate the figures, so these judgments hold on every layer of
## every stack.
check-e2e-coverage-record:
	go run ./cmd/audit_e2e_coverage/ -check-record

## check-e2e-coverage-page: compare docs/development/testing/e2e-coverage.md
## with a fresh rendering of the committed record. This is the half of the gate
## the source tree can move -- the renderer and the table formatter are code
## here -- so it is redrawn by e2e-coverage-record-render, which update-all
## runs, and CI defers it below a stack's tip exactly as it does
## check-bench-resources.
check-e2e-coverage-page:
	go run ./cmd/audit_e2e_coverage/ -check-record-page

## e2e-go-coverage: report which statements of the server binary the e2e runs
## executed, from the counter files a COVER=1 run left under $(E2E_COVER_ROOT):
## the merged profile in $(E2E_COVER_PROFILE), the per-package table and the
## total. Its sibling above answers which catalog actions dispatched, which
## says nothing about the program around them.
#
# It reads whatever runs are there, so a CE run and a licensed one merge into
# one figure, which is the honest one: a statement only the licensed catalog
# reaches was still reached. Nothing here is committed and nothing gates on
# the number: the input exists only after a live GitLab has been driven, so a
# floor could not be moved without booting one, and the profile must never be
# folded into coverage.out, which sonar-project.properties publishes as the
# unit suite's figure. The run is refused rather than reported as zero when no
# meta-data file is there at all, since that is the shape of an uninstrumented
# build and not of a binary nothing executed.
e2e-go-coverage:
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c 'set -eu; \
	  dirs=""; \
	  for meta in $$(find $(E2E_COVER_ROOT) -name "covmeta.*" -type f 2>/dev/null | sort); do \
	    dir=$$(dirname "$$meta"); \
	    case ",$$dirs," in *",$$dir,"*) continue;; esac; \
	    dirs="$${dirs:+$$dirs,}$$dir"; \
	  done; \
	  if [ -z "$$dirs" ]; then \
	    echo "no coverage data under $(E2E_COVER_ROOT): run an e2e target with COVER=1, which builds the server with -cover and gives every child a GOCOVERDIR" >&2; \
	    exit 1; \
	  fi; \
	  echo "merging $$dirs"; \
	  go tool covdata textfmt -i="$$dirs" -o=$(E2E_COVER_PROFILE); \
	  go tool covdata percent -i="$$dirs"; \
	  go tool cover -func=$(E2E_COVER_PROFILE) | tail -n 1'

## check-e2e-static: the push-time gate over the new e2e suite, with no
## GitLab: every typed action id names a catalog action, sits in a package
## that can run it, and an Ultimate id in ee declares its tier; a harness
## result thrown away is a finding. With the ratchet on it holds the other
## direction too: a catalog action no scenario names and no exemption
## declares fails, and so does a harness export nothing consumes.
check-e2e-static:
	go run ./cmd/audit_e2e_coverage/ -static

## audit-test-goroutines: report testing.T aborts made off the test goroutine
## (t.Fatal inside HTTP mock handlers, go statements, MCP tool handlers) and
## t.Errorf calls without the contract-required return. Writes the JSON work
## list consumed by the conversion batches.
audit-test-goroutines:
	go run ./cmd/audit_test_goroutines/ -json plan/test-goroutines-backlog.json

## check-test-goroutines: fail when any testing.T abort remains off the test
## goroutine. Wired into CI once the sweep lands (phase 4 of the plan).
check-test-goroutines:
	go run ./cmd/audit_test_goroutines/ -check

## audit-test-subtests: report case loops in Test functions that assert
## without opening a t.Run subtest (the table-driven rule), with the JSON work
## list. `go run ./cmd/audit_test_subtests/ -fix` rewrites the unambiguous
## sites; a `// sequential:` comment above a loop declares dependent steps.
audit-test-subtests:
	go run ./cmd/audit_test_subtests/ -json plan/test-subtests-backlog.json

## check-test-subtests: fail when any case loop still asserts without a
## subtest. CI gate.
check-test-subtests:
	go run ./cmd/audit_test_subtests/ -check

## audit-md-escaping: report every value a Markdown formatter interpolates into
## a table cell, a heading, a list item, a link or a hand-written code fence
## without routing it through toolutil.EscapeMdTableCell, EscapeMdHeading,
## MdTitleLink or MarkdownFencedBlock, with the JSON work list. The two staged
## rules are named beside `all`: `card` lists every card row written by hand
## instead of through toolutil.Card, and `bool-time` every flag or timestamp
## printed without BoolEmoji or FormatTime; both report here and do not gate
## yet. `//gitlab:allow-unescaped <expression>: <reason>` in the owning
## package declares a value that needs no escaping, and
## `//gitlab:allow-raw <expression>: <reason>` one whose raw form is right.
audit-md-escaping:
	go run ./cmd/audit_md_escaping/ -v -contexts all,card,bool-time -json plan/md-escaping-backlog.json

## check-md-escaping: fail when a value still reaches a Markdown construct
## unescaped, when a directive excuses nothing, or when internal/toolutil holds
## a value the audit cannot follow, since a blind spot there sits behind every
## formatter that calls it. The staged rules are not judged here. CI gate.
check-md-escaping:
	go run ./cmd/audit_md_escaping/ -check -fail-unresolved-in internal/toolutil

## audit-action-ids: report every canonical action ID this repository publishes
## to a model that the catalog does not have: the RelatedActions of an
## ActionSpec, the first argument of every toolutil.HintAction call, and a
## dotted ID spelled inside a Usage line or an individual tool's description.
## Constants are folded by the type checker rather than matched as text, and
## the IDs are judged against the catalog built at Ultimate for a self-managed
## instance and for GitLab.com together, so the Orbit family does not read as
## dead. The work list lands in plan/action-ids.json.
audit-action-ids:
	go run ./cmd/audit_action_ids/ -v -json plan/action-ids.json

## check-action-ids: fail when a published ID is not a canonical catalog ID.
## The demand is the canonical ID and not mere resolvability: a registered
## alias fails too, because gitlab_execute_action resolves one and
## gitlab_find_action publishes IDs, so a cross-link spelled as an alias works
## when it is followed and can be found in no listing. A site the type checker
## could not fold fails as well, since a gate with a silent blind spot is one a
## new site can step into. CI gate.
check-action-ids:
	go run ./cmd/audit_action_ids/ -check -json ''

## audit-dead-consts: report every unexported constant in internal/ and cmd/
## that nothing reads. staticcheck's unused judges a const group as one unit,
## so a member sharing a declaration with a member that is read is never
## looked at, and that group is the prevailing shape here: the action-ID block
## every domain keeps, and the assertion-message block beside it in the tests.
audit-dead-consts:
	go run ./cmd/audit_dead_consts/ -v

## check-dead-consts: the same rule as a gate. A constant nothing reads is
## deleted by the change that introduces it, so this fails rather than
## reporting. CI gate.
check-dead-consts:
	go run ./cmd/audit_dead_consts/ -check

## audit-gateway-chars: report served descriptions and titles violating the
## gateway-safe text policy (pure ASCII prose, no semicolons), across every
## tool surface plus prompts and resources.
audit-gateway-chars:
	go run ./cmd/audit_gateway_chars/

## check-gateway-chars: fail when anything served carries an offending
## character, so a rejection at a gateway's door cannot ship silently.
check-gateway-chars:
	go run ./cmd/audit_gateway_chars/ -check

## audit-meta-descriptions: report every parameter or value a served meta-tool
## description offers that its actions do not accept, and name every line
## describing an action's parameters that the extraction rule could not read,
## since a line outside the check is where a stale parameter hides.
audit-meta-descriptions:
	go run ./cmd/audit_meta_descriptions/ -uncovered

## check-meta-descriptions: fail when a served description and the schemas
## disagree. The description is read out of the same snapshot the regenerator
## writes from it, so regeneration can never notice one going stale: this is
## the third party.
check-meta-descriptions:
	go run ./cmd/audit_meta_descriptions/ -check

## check-readonly-graphql: fail when an action classified ReadOnly can reach a
## GraphQL mutation. --read-only and the surface served to a read_api token
## both keep such an action, so it would write exactly where a write is
## supposed to be impossible. The HTTP method cannot see this: client-go POSTs
## every GraphQL request, reads included.
check-readonly-graphql:
	go run ./cmd/audit_readonly_graphql/

## audit-readonly-graphql: same gate, listing the read-only actions that touch
## GraphQL at all rather than only what failed.
audit-readonly-graphql:
	go run ./cmd/audit_readonly_graphql/ -v

## gen-graphql-schema: re-pin the GitLab GraphQL schema from a live instance.
## Needs the network, so it is not a gate: run it when GitLab has changed and
## commit the result. Set GITLAB_TOKEN to a gitlab.com credential: GitLab
## answers introspection to anyone but tells only an authenticated caller which
## version it runs, and check-graphql-schema refuses a pin that records none.
gen-graphql-schema:
	go run ./cmd/gen_graphql_schema/

## check-graphql-schema: fail when the committed schema does not parse, its
## provenance record does not decode, or the pin is not of what this project
## claims to be pinned to: another instance, a truncated or narrower answer, no
## recorded version, or past the shared staleness window
## (cmd/internal/provenance). No network, so it is a gate.
check-graphql-schema:
	go run ./cmd/gen_graphql_schema/ --check

## gen-api-live: boot a released GitLab image, ask the loaded application what
## its REST API is, and rewrite docs/development/gitlab-api-live.json. It needs
## Docker and takes a few minutes on a cold image, about forty seconds
## afterwards; it needs no licence and no fixtures, because a licence gates
## feature_available? when a request is served and not when a class is defined.
## This is the only oracle here that is evaluated rather than parsed, which is
## what lets it see the ~600 fields GeoSiteStatus exposes through a loop where
## a scan of the same source sees 26.
gen-api-live:
	go run ./cmd/gen_api_live/

## check-api-live: fail when the committed live record is not one this build can
## read, is too small to have come from a GitLab, holds an entity that refused to
## describe itself, or is past the shared staleness window
## (cmd/internal/provenance). No Docker and no network, so it is a gate.
check-api-live:
	go run ./cmd/gen_api_live/ -check

## check-graphql-documents: fail when a raw GraphQL document in the source is
## one GitLab would refuse. The test transport catches the documents a test
## sends; this catches the ones no test reaches.
check-graphql-documents:
	go run ./cmd/audit_graphql_documents/

## audit-graphql-documents: same gate, listing every document checked rather
## than only the refused ones.
audit-graphql-documents:
	go run ./cmd/audit_graphql_documents/ -v

## check-graphql-shapes: fail when a struct a GraphQL response is decoded into
## cannot hold what its document selects, or declares a field the document
## never selects. Every other GraphQL gate reads the request; this one pairs
## each document with its decoder and reads the answer.
check-graphql-shapes:
	go run ./cmd/audit_graphql_shapes/

## audit-graphql-shapes: same gate, listing every pairing judged and every
## selection nothing reads.
audit-graphql-shapes:
	go run ./cmd/audit_graphql_shapes/ -v

## audit-graphql-sent: the reverse question, written to plan/graphql-sent.json.
## What does the pinned schema offer at an object one of our decoders reads that
## no document of the package decoding it ever selects? It is the sent question
## R-PATH asks of REST against GitLab's OpenAPI record, asked of GraphQL, where
## the eleven GraphQL-only domains contribute nothing to that record because
## their output types pair with no client-go struct. The claim is package-wide
## on purpose: a CE and an EE document of one package differ deliberately, and
## judging each alone reports the sibling's selections as gaps. It REPORTS and
## does not gate: a field GitLab offers and this server does not surface is a
## candidate for the surface, and this dimension has no tier and no deprecation
## oracle to sort the candidates with, both of which the report states. It reads
## ./internal/... like check-graphql-documents, so the operations client-go
## builds are outside it, and the report names that set from the request
## inventory rather than leaving it unsaid. The one sub-class that gates, a
## mutation payload whose errors no field of the decoder reads, gates in the
## shape check above and so on every run. The record is deliberately not
## committed and not freshness-gated: a schema re-pin would churn it every time.
audit-graphql-sent:
	@mkdir -p plan
	go run ./cmd/audit_graphql_shapes/ -report plan/graphql-sent.json

## check-graphql-documents-live: judge every document against a schema fetched
## from a live instance right now rather than the pinned one, and report where
## the pin and that instance disagree about a type or field the documents touch.
## Needs the network, so it is not a CI gate; it is the only thing that reports
## a field GitLab has narrowed since the pin, which is how every defect the pin
## was built for arose. Run it beside the other live suites
## (make test-e2e-gitlab-com). GRAPHQL_LIVE_URL points it somewhere else: the
## scheduled ee-schema workflow points it at an unlicensed GitLab EE, which
## serves the whole Enterprise schema because GitLab builds the schema at boot,
## before any license is applied.
GRAPHQL_LIVE_URL ?= https://gitlab.com/api/graphql
check-graphql-documents-live:
	go run ./cmd/audit_graphql_documents/ -live "$(GRAPHQL_LIVE_URL)"

## record-request-inventory: run the unit suite with request recording on,
## leaving one shard per test process under dist/request-inventory. This is
## where the minutes go; merging them is instant, which is why recording is a
## target of its own and the two targets below consume what it leaves.
## Recording is off unless GITLAB_MCP_TEST_INVENTORY_DIR names an absolute
## directory, so an ordinary `make test` pays nothing for it.
##
## A suite that fails takes the shards with it and says so on the way out.
## That is the whole reason this is written as a shell branch rather than one
## command: `make gen-request-inventory` used to merge only after a passing
## run, so a failure rewrote nothing, `git status` stayed clean, and the tree
## read as already current when nothing had been recorded at all. A benchmark
## test that lost a port it had bound decided whether the inventory was
## regenerated, silently. Removing the shards is what makes the merge that
## follows refuse outright rather than publish half a run as the whole answer.
##
## The R-PATH gate's own unit test skips while that variable is set (see
## TestRun_TheRealTree_PassesItsOwnGate): without that, a new domain package
## could not be recorded, because the gate failed on the package the recording
## was about to add and make stopped before merging.
record-request-inventory:
	$(call RM_RF,$(REQUEST_INVENTORY_SHARDS))
	$(call MKDIR_P,$(REQUEST_INVENTORY_SHARDS))
	@GITLAB_MCP_TEST_INVENTORY_DIR=$(CURDIR)/$(REQUEST_INVENTORY_SHARDS) go test -count=1 $(PKGS) || { \
		$(call RM_RF,$(REQUEST_INVENTORY_SHARDS)); \
		echo ''; \
		echo 'NOT RECORDED: the unit suite failed, so no request was written down.'; \
		echo 'docs/development/request-inventory.json was left exactly as it was: a clean'; \
		echo 'git status here means nothing ran, not that the inventory is current.'; \
		echo 'Fix the suite and run this again.'; \
		exit 1; \
	}

## gen-request-inventory: record the suite, then rewrite
## docs/development/request-inventory.json from what that run recorded.
gen-request-inventory: record-request-inventory
	go run ./cmd/gen_request_inventory/

## check-request-inventory: fail when the committed request inventory is not
## what the last recorded run says. It merges the shards already under
## dist/request-inventory and compares, so it costs one `go run` instead of a
## suite of its own, and refuses with a message naming the recording target
## when no run has left any. Recording stays opt-in: `make
## record-request-inventory`, or `make gen-request-inventory` to record and
## rewrite in one go.
##
## What it answers is therefore "is the committed inventory what that run
## recorded", which is a statement about this tree only when the recording was
## made from this tree; the summary line prints when the shards were written,
## so an older recording is visible rather than assumed. CI gets both halves
## for nothing: the coverage job records on the suite run it was going to do
## anyway and merges those shards afterwards.
check-request-inventory:
	go run ./cmd/gen_request_inventory/ -check

## audit-request-inventory: merge the shards of the last recorded run and name
## every package the catalog owns actions in that issued no request at all.
audit-request-inventory:
	go run ./cmd/gen_request_inventory/ -v -check

## audit-test-names: audit test function naming convention compliance.
audit-test-names:
	go run ./cmd/audit_test_names/ cmd internal test

## check-test-file-names: fail when a _test.go file is not named after a
## module it tests (export_test.go, build-constrained and external-package
## qualifiers, and test/e2e are the codified exemptions).
check-test-file-names:
	go run ./cmd/audit_test_names/ -check-files cmd internal test

## check-supply-chain: fail when a release-configuration invariant nothing else
## in the pipeline can see has broken — an unpinned uses:, a credentialed job
## that runs code resolved at run time, a dropped Dependabot cooldown, a stale
## security policy, or an installer that stopped verifying the signature.
check-supply-chain:
	go run ./cmd/audit_supply_chain/

## check-em-dash: fail when a line this branch adds carries an em dash
## (U+2014). Scoped to the added lines of EM_DASH_BASE...HEAD, net over the
## branch rather than per commit, so a branch that adds one and cleans it up at
## its tip passes. It has to be scoped: the tree already carries thousands in
## prose written before the rule, so a whole-tree check would be red on its
## first run and switched off on its second. No network, no Go build, so it is
## a gate; run it before pushing. EM_DASH_BASE defaults to origin/main.
check-em-dash:
	scripts/check-em-dash.sh diff $(EM_DASH_BASE)

## check-pr-description: fail when the pull request title or body carries an em
## dash, or when the body carries a block a review bot injected. This is the
## half no later commit can fix: a squash merge copies the description into
## main's history. A branch with no pull request open passes, having no
## description to land; PR_NUMBER names one explicitly. Without gh it fails
## and says so, because a check that cannot read what it judges must not
## report that it judged it. PR_TITLE_FILE and PR_BODY_FILE judge text from
## disk instead, which needs no gh and is how the gate is rehearsed.
check-pr-description:
	scripts/check-em-dash.sh description

## audit-godocs: generate a Godoc compliance report, including test functions.
audit-godocs:
	$(call MKDIR_P,$(ANALYSIS_DIR))
	go run ./cmd/godoc_tool/ audit --include-tests --format=markdown --output=$(ANALYSIS_DIR)/godoc.md
	@echo "Godoc report saved to $(ANALYSIS_DIR)/godoc.md"

## audit-godocs-check: fail when package, symbol, or test Godoc findings remain.
audit-godocs-check:
	go run ./cmd/godoc_tool/ audit --include-tests --fail-on-findings

## fix-godocs: generate and insert godoc-compliant comments for the given paths.
## Use --dry-run to preview changes without writing (e.g. make fix-godocs ARGS="--dry-run internal/tools/").
fix-godocs:
	go run ./cmd/godoc_tool/ fix $(ARGS)

## docs-local-go: serve local pkg.go.dev-style documentation at http://127.0.0.1:6060.
docs-local-go:
	@if [ ! -x "$(PKGSITE)" ]; then echo "pkgsite not found. Install with: go install golang.org/x/pkgsite/cmd/pkgsite@latest"; exit 1; fi
	@echo "Serving local Go documentation at http://127.0.0.1:6060"
	$(PKGSITE) -http=127.0.0.1:6060

# ─── Formatting ──────────────────────────────────────────────────────────────
# Formatting is delegated to golangci-lint formatters configured in .golangci.yml.

## fmt: apply configured Go formatters.
fmt:
	golangci-lint fmt

## release: build release binaries using GoReleaser (local snapshot, no publish).
## Produces flat binaries in dist/ matching GitHub Release asset names.
release:
	goreleaser release --snapshot --clean
	@# Flatten dist/: move binaries out of subdirs, remove GoReleaser metadata
	@for dir in dist/gitlab-mcp-server_*; do \
		if [ -d "$$dir" ]; then \
			os_arch=$$(echo "$$dir" | sed -E 's|dist/gitlab-mcp-server_([^_]+)_([^_]+).*|\1-\2|'); \
			src=$$(find "$$dir" -maxdepth 1 -type f | head -1); \
			if echo "$$src" | grep -q '\.exe$$'; then \
				mv "$$src" "dist/gitlab-mcp-server-$${os_arch}.exe"; \
			else \
				mv "$$src" "dist/gitlab-mcp-server-$${os_arch}"; \
			fi; \
			rm -rf "$$dir"; \
		fi; \
	done
	@rm -f dist/artifacts.json dist/config.yaml dist/metadata.json
	@echo "dist/ contents:" && ls -1 dist/

## release-check: validate .goreleaser.yml configuration
release-check:
	goreleaser check

checksum:
	@cat dist/checksums.txt

# ─── MCP Inspector ───────────────────────────────────────────────────────────
# Requires: Node.js >= 22, npx, .env with GITLAB_TOKEN. Add GITLAB_URL for self-managed instances.
# Compiles a fresh binary to /tmp, launches the Inspector, and cleans up on exit.

INSPECTOR_BIN := /tmp/$(BINARY_NAME)-inspector$(BINARY_EXT)

## inspector: compile the server and launch MCP Inspector UI via stdio.
## Reads credentials from .env. The temporary binary is removed on exit.
inspector:
	@if [ ! -f .env ]; then echo "ERROR: .env file not found. Create it with GITLAB_TOKEN; add GITLAB_URL for self-managed instances."; exit 1; fi
	@echo "Compiling $(BINARY_NAME) to $(INSPECTOR_BIN)..."
	@go build -ldflags="$(LDFLAGS)" -o $(INSPECTOR_BIN) $(CMD_PATH)
	@echo "Starting MCP Inspector (stdio) — press Ctrl+C to stop..."
	@trap 'rm -f $(INSPECTOR_BIN); echo "Cleaned up $(INSPECTOR_BIN)"' EXIT INT TERM && \
		set -a && . ./.env && set +a && \
		ALLOWED_ORIGINS="http://localhost:6274,http://127.0.0.1:6274,http://0.0.0.0:6274" \
		HOST=0.0.0.0 \
		npx -y @modelcontextprotocol/inspector \
			-e GITLAB_URL="$${GITLAB_URL:-https://gitlab.com}" \
			-e GITLAB_TOKEN="$$GITLAB_TOKEN" \
			-e GITLAB_MCP_SKIP_TLS_VERIFY="$${GITLAB_MCP_SKIP_TLS_VERIFY:-false}" \
			-e GITLAB_MCP_TOOL_SURFACE=meta \
			-- $(INSPECTOR_BIN)

## inspector-stop: stop any running MCP Inspector and server processes.
inspector-stop:
	@pkill -f "@modelcontextprotocol/inspector" 2>/dev/null || true
	@pkill -f "node.*inspector" 2>/dev/null || true
	@rm -f $(INSPECTOR_BIN)
	@echo "MCP Inspector stopped."

clean:
	$(call RM_RF,dist)
	$(call RM_F,coverage.out)
	$(call RM_F,coverage.html)

## help: show available targets
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
