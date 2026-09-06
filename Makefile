.PHONY: build build-all build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64 \
	run brand brand-check brand-rasters ensure-mcp-publisher mcp-publisher-version test test-short test-race coverage-conditions coverage-mutants test-pkg test-integration test-e2e test-e2e-http test-e2e-stdio test-e2e-collector ensure-gotestsum test-e2e-docker test-e2e-docker-enterprise test-e2e-gitlab-com \
	validate-http-stateless validate-http-stateless-docker \
	orbit-setup-fixtures orbit-wait-indexer orbit-run-live-tests orbit-ensure-token \
	eval-surfaces-docker eval-surfaces-docker-enterprise eval-surfaces-docker-enterprise-ce eval-surfaces-docker-enterprise-all eval-surfaces-docker-enterprise-all-fixtures coverage \
	lint fmt clean version release release-check checksum \
	golangci-lint govulncheck sonar sonar-status \
	mdlint mdlint-fix audit-docs check-doc-links \
	analyze analyze-fix analyze-report install-tools \
	audit-output audit-tokens audit-tools audit-surface-quality audit-metrics audit-dynamic-aliases audit-test-names audit-godocs audit-godocs-check fix-godocs \
	audit-struct-completeness audit-action-coverage audit-metadata-completeness audit-1to1 audit-1to1-sdk audit-1to1-enums audit-1to1-validate-docs audit-edition-tier \
	audit-discovery audit-discovery-check audit-e2e-gaps audit-gateway-chars check-gateway-chars check-test-file-names audit-test-subtests check-test-subtests check-supply-chain \
	check-readonly-graphql audit-readonly-graphql \
	audit-doc-coverage audit-doc-coverage-check \
	gen-action-catalog-manifest check-action-catalog-manifest gen-llms check-llms gen-lhm-manifest check-lhm-manifest gen-icon-webp check-icon-webp check-server-json check-server-json-packages check-openplugin audit-doc-tool-names check-doc-tool-names check-install-buttons check-mcpb mcpb gen-npm sync-npm-version validate-npm validate-npm-local publish-npm-dry publish-npm gen-pypi validate-pypi validate-pypi-local publish-pypi-dry publish-pypi gen-nuget validate-nuget validate-nuget-local publish-nuget-dry publish-nuget publish-lobehub gen-readme gen-footprint check-footprint gen-stats check-stats gen-site-stats check-site-stats gen-testing-docs check-testing-docs update-all \
	bench-resources bench-resources-render check-bench-resources \
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
# list lives in cmd/gen_testing_docs as e2eTags, for the same reason.
GO_ANALYSIS_TAGS=e2e,collectore2e,httpe2e,orbitlive,stdioe2e
PROJECT_GO_VERSION := $(shell awk '/^go / {print $$2; exit}' go.mod)
GO_TOOLCHAIN ?= go$(PROJECT_GO_VERSION)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)

# E2E test report directory (inside dist/, gitignored)
E2E_REPORT_DIR=dist/e2e-reports

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
E2E_DOCKER_ENTERPRISE_TIMEOUT ?= 3600s

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

## test-e2e: run end-to-end tests against a real GitLab instance (reads GITLAB_URL, GITLAB_TOKEN from .env)
test-e2e: ensure-gotestsum
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c '$(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-junit.xml \
	  -- -tags e2e -count=1 -timeout 300s ./test/e2e/suite/'

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

## test-e2e-stdio: run the stdio transport end-to-end module (no GitLab needed; drives the real binary over pipes).
test-e2e-stdio: ensure-gotestsum
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c '$(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-stdio-junit.xml \
	  -- -tags stdioe2e -count=1 -timeout 900s ./test/e2e/stdio/'

## test-e2e-http: run the HTTP transport end-to-end module (no GitLab needed; nginx layer skips without Docker).
test-e2e-http: ensure-gotestsum
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c '$(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-http-junit.xml \
	  -- -tags httpe2e -count=1 -timeout 900s ./test/e2e/http/'

## test-e2e-collector: run this server's telemetry into a real OpenTelemetry Collector (Docker required; skips cleanly without it).
# Deliberately absent from the push-triggered CI jobs, which run the httpe2e and
# stdioe2e tags and never this one. Those two need no daemon; this one pulls a
# container image, which belongs with the Docker-mode targets rather than in the
# path every commit waits on.
test-e2e-collector: ensure-gotestsum
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c '$(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-collector-junit.xml \
	  -- -tags collectore2e -count=1 -timeout 900s ./test/e2e/collector/'

## validate-http-stateless: smoke-validate stateless streamable HTTP with the compiled binary (reads GITLAB_URL, GITLAB_TOKEN from .env)
validate-http-stateless:
	scripts/validate-http-stateless.sh binary

## validate-http-stateless-docker: smoke-validate stateless streamable HTTP with the Docker image
validate-http-stateless-docker:
	scripts/validate-http-stateless.sh docker

## test-e2e-docker: start ephemeral GitLab CE (+ Bitbucket fixture), run E2E tests, tear down
test-e2e-docker: ensure-gotestsum
	@echo "=== Cleaning up previous containers (if any) ==="
	docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v 2>/dev/null || true
	@echo "=== Starting ephemeral GitLab CE and Bitbucket fixture ==="
	@openssl rand -hex 16 > test/e2e/.bitbucket-admin-pass
	E2E_BITBUCKET_ADMIN_PASSWORD=$$(cat test/e2e/.bitbucket-admin-pass) docker compose -f test/e2e/docker-compose.yml --profile bitbucket up -d
	@echo "=== Waiting for GitLab readiness ==="
	./test/e2e/scripts/wait-for-gitlab.sh http://localhost:8929 600
	@echo "=== Setting up test user and token ==="
	@set -e; \
	for attempt in 1 2 3; do \
		if ./test/e2e/scripts/setup-gitlab.sh http://localhost:8929; then \
			break; \
		fi; \
		if [ "$$attempt" -eq 3 ]; then \
			echo "ERROR: setup-gitlab.sh failed after 3 attempts"; \
			exit 1; \
		fi; \
		echo "WARN: setup-gitlab.sh failed (attempt $$attempt/3), retrying in 5s..."; \
		sleep 5; \
	done
	@echo "=== Registering GitLab Runner ==="
	./test/e2e/scripts/register-runner.sh http://localhost:8929
	@echo "=== Provisioning Bitbucket import fixture ==="
	E2E_BITBUCKET_ADMIN_PASSWORD=$$(cat test/e2e/.bitbucket-admin-pass) ./test/e2e/scripts/setup-bitbucket.sh http://localhost:7990
	@echo "=== Running E2E tests ==="
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	@set +e; \
	  bash -o pipefail -c 'set -a && . test/e2e/.env.docker && set +a && E2E_MODE=docker $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-docker-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-docker-log.json \
	  -- -tags e2e -timeout 1800s ./test/e2e/suite/ 2>&1 | tee $(E2E_REPORT_DIR)/e2e-docker-output.txt'; \
	  echo $$? > $(E2E_REPORT_DIR)/e2e-docker-status
	@echo "=== Tearing down ==="
	@status=$$(cat $(E2E_REPORT_DIR)/e2e-docker-status); \
	  teardown_status=0; \
	  docker compose -f test/e2e/docker-compose.yml --profile bitbucket down -v || teardown_status=$$?; \
	  echo "=== E2E reports saved to $(E2E_REPORT_DIR)/ ==="; \
	  rm -f $(E2E_REPORT_DIR)/e2e-docker-status; \
	  if [ "$$status" -ne 0 ]; then exit "$$status"; fi; \
	  if [ "$$teardown_status" -ne 0 ]; then exit "$$teardown_status"; fi

## test-e2e-docker-enterprise: start ephemeral GitLab EE with cached license, ENTERPRISE_LICENSE, or GITLAB_ACTIVATION_CODE, run E2E tests, tear down
test-e2e-docker-enterprise: ensure-gotestsum
	@echo "=== Cleaning up previous containers (if any) ==="
	GITLAB_IMAGE=$${GITLAB_IMAGE:-gitlab/gitlab-ee:latest} docker compose -f test/e2e/docker-compose.yml down -v 2>/dev/null || true
	@echo "=== Starting ephemeral GitLab EE ==="
	@activation_code="$$(./test/e2e/scripts/enterprise-activation-code.sh)"; \
	  if [ -n "$$activation_code" ]; then echo "    Passing Enterprise activation code to GitLab EE container"; fi; \
	  if [ -z "$$activation_code" ] && [ -s "$${E2E_ENTERPRISE_LICENSE_FILE:-test/e2e/.enterprise-license}" ]; then echo "    Reusing cached Enterprise license during setup"; fi; \
	  GITLAB_IMAGE=$${GITLAB_IMAGE:-gitlab/gitlab-ee:latest} GITLAB_ACTIVATION_CODE="$$activation_code" docker compose -f test/e2e/docker-compose.yml up -d
	@echo "=== Waiting for GitLab readiness ==="
	./test/e2e/scripts/wait-for-gitlab.sh http://localhost:8929 600
	@echo "=== Setting up test user, token, and Enterprise license ==="
	@set -e; \
	for attempt in 1 2 3; do \
		if GITLAB_ENTERPRISE=true ./test/e2e/scripts/setup-gitlab.sh http://localhost:8929; then \
			break; \
		fi; \
		if [ "$$attempt" -eq 3 ]; then \
			echo "ERROR: setup-gitlab.sh failed after 3 attempts"; \
			exit 1; \
		fi; \
		echo "WARN: setup-gitlab.sh failed (attempt $$attempt/3), retrying in 5s..."; \
		sleep 5; \
	done
	@echo "=== Registering GitLab Runner ==="
	./test/e2e/scripts/register-runner.sh http://localhost:8929
	@echo "=== Running Enterprise E2E tests ==="
	@$(call MKDIR_P,$(E2E_REPORT_DIR))
	@set +e; \
	  bash -o pipefail -c 'set -a && . test/e2e/.env.docker && set +a && \
	  echo "Enterprise E2E suite: build tags e2e,enterprise"; \
	  E2E_MODE=docker $(GOTESTSUM) \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-docker-enterprise-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-docker-enterprise-log.json \
	  -- -tags "e2e enterprise" -timeout $(E2E_DOCKER_ENTERPRISE_TIMEOUT) ./test/e2e/suite/ \
	  2>&1 | tee $(E2E_REPORT_DIR)/e2e-docker-enterprise-output.txt'; \
	  echo $$? > $(E2E_REPORT_DIR)/e2e-docker-enterprise-status
	@echo "=== Tearing down ==="
	@status=$$(cat $(E2E_REPORT_DIR)/e2e-docker-enterprise-status); \
	  teardown_status=0; \
	  GITLAB_IMAGE=$${GITLAB_IMAGE:-gitlab/gitlab-ee:latest} docker compose -f test/e2e/docker-compose.yml down -v || teardown_status=$$?; \
	  echo "=== E2E reports saved to $(E2E_REPORT_DIR)/ ==="; \
	  rm -f $(E2E_REPORT_DIR)/e2e-docker-enterprise-status; \
	  if [ "$$status" -ne 0 ]; then exit "$$status"; fi; \
	  if [ "$$teardown_status" -ne 0 ]; then exit "$$teardown_status"; fi

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
test-e2e-gitlab-com: orbit-ensure-token orbit-setup-fixtures orbit-wait-indexer orbit-run-live-tests
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

## eval-surfaces-docker: run Docker CE model evaluation for one surface (usage: make eval-surfaces-docker SURFACE=dynamic [PRESET=docker-read] [SERVER_MODE=read-only|safe-mode])
eval-surfaces-docker:
	@if [ -z "$(SURFACE)" ]; then echo "Usage: make eval-surfaces-docker SURFACE=dynamic|meta" >&2; exit 1; fi
	@if [ -n "$(PRESET)" ]; then \
		./scripts/eval-surfaces-docker.sh "$(SURFACE)" "$(PRESET)"; \
	else \
		./scripts/eval-surfaces-docker.sh "$(SURFACE)"; \
	fi

## eval-surfaces-docker-enterprise: run Docker Enterprise model evaluation for one surface (usage: make eval-surfaces-docker-enterprise SURFACE=dynamic)
eval-surfaces-docker-enterprise:
	@if [ -z "$(SURFACE)" ]; then echo "Usage: make eval-surfaces-docker-enterprise SURFACE=dynamic|meta" >&2; exit 1; fi
	@if [ -n "$(PRESET)" ]; then \
		EVAL_SURFACE_ENTERPRISE=true ./scripts/eval-surfaces-docker.sh "$(SURFACE)" "$(PRESET)"; \
	else \
		EVAL_SURFACE_ENTERPRISE=true ./scripts/eval-surfaces-docker.sh "$(SURFACE)"; \
	fi

## eval-surfaces-docker-enterprise-ce: run CE evaluation cases against Docker Enterprise runtime (usage: make eval-surfaces-docker-enterprise-ce SURFACE=dynamic)
eval-surfaces-docker-enterprise-ce:
	@if [ -z "$(SURFACE)" ]; then echo "Usage: make eval-surfaces-docker-enterprise-ce SURFACE=dynamic|meta" >&2; exit 1; fi
	@if [ -n "$(PRESET)" ]; then \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=ce ./scripts/eval-surfaces-docker.sh "$(SURFACE)" "$(PRESET)"; \
	else \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=ce ./scripts/eval-surfaces-docker.sh "$(SURFACE)"; \
	fi

## eval-surfaces-docker-enterprise-all: run CE and Enterprise evaluation cases against Docker Enterprise runtime (usage: make eval-surfaces-docker-enterprise-all SURFACE=dynamic)
eval-surfaces-docker-enterprise-all:
	@if [ -z "$(SURFACE)" ]; then echo "Usage: make eval-surfaces-docker-enterprise-all SURFACE=dynamic|meta" >&2; exit 1; fi
	@if [ -n "$(PRESET)" ]; then \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=all ./scripts/eval-surfaces-docker.sh "$(SURFACE)" "$(PRESET)"; \
	else \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=all ./scripts/eval-surfaces-docker.sh "$(SURFACE)"; \
	fi

## eval-surfaces-docker-enterprise-all-fixtures: prepare and smoke-test CE+Enterprise fixtures against Docker Enterprise runtime without model calls (usage: make eval-surfaces-docker-enterprise-all-fixtures SURFACE=dynamic)
eval-surfaces-docker-enterprise-all-fixtures:
	@if [ -z "$(SURFACE)" ]; then echo "Usage: make eval-surfaces-docker-enterprise-all-fixtures SURFACE=dynamic|meta" >&2; exit 1; fi
	@if [ -n "$(PRESET)" ]; then \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=all EVAL_SURFACE_FIXTURE_SMOKE=true ./scripts/eval-surfaces-docker.sh "$(SURFACE)" "$(PRESET)"; \
	else \
		EVAL_SURFACE_ENTERPRISE=true EVAL_SURFACE_CASE_SET=all EVAL_SURFACE_FIXTURE_SMOKE=true ./scripts/eval-surfaces-docker.sh "$(SURFACE)"; \
	fi

## coverage-conditions: report the boolean conditions of PKG never evaluated both ways (gobco). A line reported is a missing test case; `&&`, `||` and `!` operands count separately.
coverage-conditions:
	@test -n "$(PKG)" || { echo "usage: make coverage-conditions PKG=./cmd/gen_stats"; exit 2; }
	cd $(PKG) && go run github.com/rillig/gobco@v1.3.4

## coverage-mutants: mutation-test PKG with gremlins, INVERT_LOGICAL on so `&&`/`||` independence is checked. The gate on a changed package is Lived 0 and Not covered 0.
coverage-mutants:
	@test -n "$(PKG)" || { echo "usage: make coverage-mutants PKG=./cmd/gen_stats"; exit 2; }
	go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 unleash --invert-logical --workers 4 $(GREMLINS_FLAGS) $(PKG)

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

## check-doc-links: verify tracked Markdown/MDX local links resolve.
check-doc-links:
	@echo === documentation local links ===
	node scripts/check-doc-links.mjs

## audit-docs: run the complete documentation quality gate.
audit-docs:
	npx markdownlint-cli2 README.md AGENTS.md CLAUDE.md CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md "docs/**/*.md" "test/e2e/**/*.md" "cmd/eval_mcp_surfaces/**/*.md" "site/src/content/docs/**/*.mdx" "site/src/content/i18n/**/*.md"
	go run ./cmd/format_md_tables/ --check
	go run ./cmd/gen_llms/ --check
	go run ./cmd/gen_lhm_manifest/ --check
	$(MAKE) check-testing-docs
	go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json -check
	$(MAKE) check-doc-links
	go run ./cmd/godoc_tool/ audit
	go run ./cmd/audit_surface_quality/ -view=metadata
	go run ./cmd/audit_dynamic_aliases/
	go run ./cmd/audit_surface_quality/ -view=output
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
	run_check "[1/8] golangci-lint config verify" golangci-lint config verify; \
	run_check "[2/8] golangci-lint fmt" golangci-lint fmt --diff; \
	run_check "[3/8] golangci-lint run" golangci-lint run --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[4/8] govulncheck" ./scripts/govulncheck.sh -tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[5/8] markdownlint" npx markdownlint-cli2 "**/*.md" "#plan"; \
	run_check "[6/8] test-goroutine aborts" go run ./cmd/audit_test_goroutines --check; \
	run_check "[7/8] case loops without subtests" go run ./cmd/audit_test_subtests --check; \
	run_check "[8/8] supply-chain policy" go run ./cmd/audit_supply_chain; \
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

## audit-doc-tool-names: report documentation that names a tool no surface registers.
audit-doc-tool-names:
	go run ./cmd/audit_doc_tool_names/

## check-doc-tool-names: fail when the documentation names a tool that does not exist.
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
## Generates: brand vectors, token footprint, repo stats, site stats, llms.txt, LobeHub manifest, testing docs, action catalog manifest, markdown table formatting.
# One generator at a time, in the recipe rather than as prerequisites: brand
# rewrites internal/toolutil/brandmark_gen.go, which the generators after it
# compile, and gen-footprint and gen-stats both rewrite README.md, so make -j
# would interleave them. brand-rasters stays out: it needs rsvg-convert and
# cwebp, which only the maintainer's machine has.
update-all:
	@for target in brand gen-footprint gen-stats gen-site-stats gen-llms gen-lhm-manifest gen-testing-docs gen-action-catalog-manifest; do \
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

## gen-testing-docs: regenerate testing.md counts and coverage tables.
## Runs unit-test coverage over ./cmd/... and ./internal/..., so it takes minutes.
## Add -skip-coverage to refresh only the counts, keeping the recorded values.
gen-testing-docs:
	go run ./cmd/gen_testing_docs/

## check-testing-docs: verify everything in testing.md that a checkout determines.
## Seconds, not minutes: it carries the recorded coverage values forward instead of
## recomputing them, because those depend on the machine (privilege, and whether
## rsvg-convert is installed) while counts and package rows do not. This is the CI
## gate. To verify the coverage values too, regenerate with gen-testing-docs and
## look at the diff.
check-testing-docs:
	go run ./cmd/gen_testing_docs/ --check -skip-coverage

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

## audit-1to1-sdk: gate every client-go service, every raw-GraphQL operation and every
## enum value on a decision (R-SERVICE/R-GRAPHQL/R-ENUM). Unlike the three candidate
## streams this one FAILS on a finding: a new SDK service that is neither called nor
## declared, a raw-GraphQL operation whose wrapper now exists and is not adjudicated,
## an SDK enum constant the surface does not offer (or a value it offers that the SDK
## does not declare), or a declaration or exemption that has gone stale.
audit-1to1-sdk:
	go run ./cmd/audit_1to1/ -scope=sdk -gaps-only

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

## audit-e2e-gaps: report catalog actions not exercised by the e2e suite (CE+EE).
audit-e2e-gaps:
	go run ./cmd/audit_e2e_gaps/

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

## audit-gateway-chars: report served descriptions and titles violating the
## gateway-safe text policy (pure ASCII prose, no semicolons), across every
## tool surface plus prompts and resources.
audit-gateway-chars:
	go run ./cmd/audit_gateway_chars/

## check-gateway-chars: fail when anything served carries an offending
## character, so a rejection at a gateway's door cannot ship silently.
check-gateway-chars:
	go run ./cmd/audit_gateway_chars/ -check

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
