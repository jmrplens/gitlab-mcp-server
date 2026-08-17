.PHONY: build build-all build-linux-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64 \
	run test test-short test-race test-pkg test-integration test-e2e test-e2e-docker test-e2e-docker-enterprise test-e2e-gitlab-com \
	validate-http-stateless validate-http-stateless-docker \
	orbit-setup-fixtures orbit-wait-indexer orbit-run-live-tests orbit-ensure-token \
	eval-surfaces-docker eval-surfaces-docker-enterprise eval-surfaces-docker-enterprise-ce eval-surfaces-docker-enterprise-all eval-surfaces-docker-enterprise-all-fixtures coverage \
	lint fmt clean version release release-check checksum \
	golangci-lint govulncheck sonar sonar-status \
	mdlint mdlint-fix audit-docs check-doc-links \
	analyze analyze-fix analyze-report install-tools \
	audit-output audit-tokens audit-tools audit-surface-quality audit-metrics audit-dynamic-aliases audit-test-names audit-godocs audit-godocs-check fix-godocs \
	audit-struct-completeness audit-action-coverage audit-metadata-completeness audit-1to1 audit-1to1-validate-docs audit-edition-tier \
	audit-discovery audit-discovery-check audit-e2e-gaps \
	audit-doc-coverage audit-doc-coverage-check \
	gen-action-catalog-manifest check-action-catalog-manifest gen-llms check-llms gen-lhm-manifest check-lhm-manifest check-server-json check-openplugin check-mcpb mcpb publish-lobehub gen-readme gen-footprint check-footprint gen-stats check-stats gen-site-stats check-site-stats gen-testing-docs update-all \
	docs-local-go \
       docker-build docker-push docker-run \
       inspector inspector-stop help

BINARY_NAME=gitlab-mcp-server
CMD_PATH=./cmd/server
PKGS=./cmd/... ./internal/...
MCP_PUBLISHER_VERSION=v1.7.9
GO_ANALYSIS_PKGS=./...
GO_ANALYSIS_TAGS=e2e
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
test-race:
	go test -v -race -coverprofile=coverage.out $(PKGS)

## test-pkg: run tests for a specific package domain (usage: make test-pkg PKG=branches)
test-pkg:
	go test -v -count=1 ./internal/tools/$(PKG)/

## test-integration: run integration tests (build tag: integration)
test-integration:
	go test -v -tags integration -coverprofile=coverage.out $(PKGS)

## test-e2e: run end-to-end tests against a real GitLab instance (reads GITLAB_URL, GITLAB_TOKEN from .env)
test-e2e:
	@echo "WARNING: This will run E2E tests against the GitLab instance configured in .env (GITLAB_URL)."
	@echo "         Tests create and delete projects, groups, users, and other resources."
	@read -p "Are you sure you want to continue? [y/N] " confirm && [ "$$confirm" = "y" ] || { echo "Aborted."; exit 1; }
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	bash -o pipefail -c 'gotestsum \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-log.json \
	  -- -tags e2e -timeout 120s ./test/e2e/suite/ 2>&1 | tee $(E2E_REPORT_DIR)/e2e-output.txt'

## validate-http-stateless: smoke-validate stateless streamable HTTP with the compiled binary (reads GITLAB_URL, GITLAB_TOKEN from .env)
validate-http-stateless:
	scripts/validate-http-stateless.sh binary

## validate-http-stateless-docker: smoke-validate stateless streamable HTTP with the Docker image
validate-http-stateless-docker:
	scripts/validate-http-stateless.sh docker

## test-e2e-docker: start ephemeral GitLab CE, run E2E tests, tear down
test-e2e-docker:
	@echo "=== Cleaning up previous containers (if any) ==="
	docker compose -f test/e2e/docker-compose.yml down -v 2>/dev/null || true
	@echo "=== Starting ephemeral GitLab CE ==="
	docker compose -f test/e2e/docker-compose.yml up -d
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
	@echo "=== Running E2E tests ==="
	$(call MKDIR_P,$(E2E_REPORT_DIR))
	@set +e; \
	  bash -o pipefail -c 'set -a && . test/e2e/.env.docker && set +a && E2E_MODE=docker gotestsum \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-docker-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-docker-log.json \
	  -- -tags e2e -timeout 1800s ./test/e2e/suite/ 2>&1 | tee $(E2E_REPORT_DIR)/e2e-docker-output.txt'; \
	  echo $$? > $(E2E_REPORT_DIR)/e2e-docker-status
	@echo "=== Tearing down ==="
	@status=$$(cat $(E2E_REPORT_DIR)/e2e-docker-status); \
	  teardown_status=0; \
	  docker compose -f test/e2e/docker-compose.yml down -v || teardown_status=$$?; \
	  echo "=== E2E reports saved to $(E2E_REPORT_DIR)/ ==="; \
	  rm -f $(E2E_REPORT_DIR)/e2e-docker-status; \
	  if [ "$$status" -ne 0 ]; then exit "$$status"; fi; \
	  if [ "$$teardown_status" -ne 0 ]; then exit "$$teardown_status"; fi

## test-e2e-docker-enterprise: start ephemeral GitLab EE with cached license, ENTERPRISE_LICENSE, or GITLAB_ACTIVATION_CODE, run E2E tests, tear down
test-e2e-docker-enterprise:
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
	  E2E_MODE=docker gotestsum \
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
	go run ./cmd/gen_testing_docs/ --check
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
	run_check "[1/6] golangci-lint config verify" golangci-lint config verify; \
	run_check "[2/6] golangci-lint fmt" golangci-lint fmt --diff; \
	run_check "[3/6] golangci-lint run" golangci-lint run --build-tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[4/6] govulncheck" ./scripts/govulncheck.sh -tags $(GO_ANALYSIS_TAGS) $(GO_ANALYSIS_PKGS); \
	run_check "[5/6] markdownlint" npx markdownlint-cli2 "**/*.md" "#plan"; \
	run_check "[6/6] test-goroutine aborts" go run ./cmd/audit_test_goroutines --check; \
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

## check-server-json: validate server.json with the official MCP Registry publisher.
check-server-json:
	go run github.com/modelcontextprotocol/registry/cmd/publisher@$(MCP_PUBLISHER_VERSION) validate server.json

## check-openplugin: validate Open Plugins manifest and MCP config files.
check-openplugin:
	scripts/check-openplugin.sh

# Pin the MCPB packer CLI for supply-chain integrity (also pinned in scripts/build-mcpb.sh).
MCPB_CLI_VERSION := 2.1.2

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

## publish-lobehub: publish the current version to the LobeHub Marketplace.
## Reads lhm.plugin.json (version kept in sync by scripts/update-server-json-sha.sh
## on each release) and posts it via the @lobehub/market-cli. Requires a one-time
## interactive `lhm login` + `lhm github connect` first — LobeHub has no
## non-interactive publish path, so this cannot run in CI. See the release process in CLAUDE.md.
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
	echo "Publishing jmrplens-gitlab-mcp-server v$$VER to LobeHub..."; \
	npx -y @lobehub/market-cli plugin publish --dir "$(CURDIR)"

## gen-readme: regenerate all managed README.md sections (token footprint + stats).
gen-readme: gen-footprint gen-stats

## update-all: run every generator and doc updater in one pass.
## Generates: token footprint, repo stats, site stats, llms.txt, testing docs, action catalog manifest, markdown table formatting.
update-all: gen-footprint gen-stats gen-site-stats gen-llms gen-lhm-manifest gen-testing-docs gen-action-catalog-manifest
	go run ./cmd/format_md_tables/
	@echo "All generators and formatters complete."

## gen-footprint: measure token footprint and write the README section + token-footprint.md.
gen-footprint:
	go run ./cmd/audit_tokens/ -footprint

## check-footprint: verify the README token-footprint section and token-footprint.md are current.
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

## gen-testing-docs: regenerate testing.md counts and coverage tables.
gen-testing-docs:
	go run ./cmd/gen_testing_docs/

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

## audit-1to1: run all three 1:1-audit gap streams (struct/action/metadata) and merge into plan/1to1-backlog.json.
## Single binary cmd/audit_1to1 consolidates the former audit_struct_completeness,
## audit_action_coverage, audit_metadata_completeness, and gen_1to1_backlog.
audit-1to1:
	go run ./cmd/audit_1to1/ -gaps-only -output plan/1to1-backlog.json
	@echo "1:1 audit backlog written to plan/1to1-backlog.json"

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

## audit-test-names: audit test function naming convention compliance.
audit-test-names:
	go run ./cmd/audit_test_names/ cmd internal test

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
			-e GITLAB_SKIP_TLS_VERIFY="$${GITLAB_SKIP_TLS_VERIFY:-false}" \
			-e AUTO_UPDATE=false \
			-e META_TOOLS=true \
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
