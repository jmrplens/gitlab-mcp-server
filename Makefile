.PHONY: build build-all run test test-short test-race test-pkg test-integration test-e2e test-e2e-docker coverage \
       lint fmt goimports goimports-check gofmt-check clean version release release-check checksum \
       vet modernize modernize-fix golangci-lint gosec staticcheck govulncheck \
       mdlint mdlint-fix \
       analyze analyze-fix analyze-report install-tools audit-output audit-tokens gen-llms gen-readme \
       docker-build docker-push docker-run \
       fly-check fly-deploy fly-deploy-release fly-status fly-logs fly-ssh fly-restart \
       inspector inspector-stop help

BINARY_NAME=gitlab-mcp-server
CMD_PATH=./cmd/server
PKGS=./cmd/... ./internal/...

# E2E test report directory (inside dist/, gitignored)
E2E_REPORT_DIR=dist/e2e-reports

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

version: build
	dist/$(BINARY_NAME)$(BINARY_EXT) --version

build:
	go build -trimpath -buildmode=pie -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)$(BINARY_EXT) $(CMD_PATH)

build-all: build-linx-amd64 build-linux-arm64 build-windows-amd64 build-windows-arm64 build-darwin-amd64 build-darwin-arm64

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
	gotestsum \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-log.json \
	  -- -tags e2e -timeout 120s ./test/e2e/suite/ 2>&1 | tee $(E2E_REPORT_DIR)/e2e-output.txt

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
	set -a && . test/e2e/.env.docker && set +a && E2E_MODE=docker gotestsum \
	  --format testdox \
	  --junitfile $(E2E_REPORT_DIR)/e2e-docker-junit.xml \
	  --jsonfile $(E2E_REPORT_DIR)/e2e-docker-log.json \
	  -- -tags e2e -timeout 1800s ./test/e2e/suite/ 2>&1 | tee $(E2E_REPORT_DIR)/e2e-docker-output.txt || true
	@echo "=== Tearing down ==="
	docker compose -f test/e2e/docker-compose.yml down -v
	@echo "=== E2E reports saved to $(E2E_REPORT_DIR)/ ==="

## coverage: run tests and generate HTML coverage report
coverage: test
	go tool cover -html=coverage.out -o coverage.html

# ─── Static Analysis (individual) ────────────────────────────────────────────
# Documentation URLs for each tool:
#   goimports      https://pkg.go.dev/golang.org/x/tools/cmd/goimports
#   gofmt          https://pkg.go.dev/cmd/gofmt
#   go vet         https://pkg.go.dev/cmd/vet
#   modernize      https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/modernize
#   golangci-lint  https://golangci-lint.run/
#   gosec          https://github.com/securego/gosec
#   staticcheck    https://staticcheck.dev/
#   govulncheck    https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
#   markdownlint   https://github.com/DavidAnson/markdownlint-cli2
# See docs/development/static-analysis.md for full documentation.

## go vet: official Go static analyzer — detects bugs, format string mismatches,
## unreachable code, shadowed variables, incorrect struct tags, etc.
## Docs: https://pkg.go.dev/cmd/vet
vet:
	@echo === go vet ===
	go vet $(PKGS)

## modernize: suggest and apply modern Go idioms (Go 1.18-1.26 features).
## Replaces deprecated patterns with slices, maps, strings, errors packages.
## Docs: https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/modernize
modernize:
	@echo === modernize ===
	modernize ./...

## modernize-fix: apply all modernization fixes automatically (writes files)
## Safe to run en masse — fixes should not change program behavior.
modernize-fix:
	@echo === modernize -fix ===
	modernize -fix ./...

## golangci-lint: meta-linter orchestrating 25+ linters via .golangci.yml.
## Includes security (gosec), style (revive), bugs (errcheck, bodyclose), etc.
## Docs: https://golangci-lint.run/
golangci-lint:
	@echo === golangci-lint ===
	golangci-lint run ./...

## gosec: OWASP-oriented security scanner with taint analysis (G1xx-G7xx).
## Detects credentials, SQL injection, path traversal, SSRF, command injection.
## Docs: https://github.com/securego/gosec
gosec:
	@echo === gosec ===
	gosec -severity medium -confidence medium -exclude-generated ./...

## staticcheck: advanced static analysis covering SA/S/ST/QF check categories.
## Finds bugs, simplifications, deprecations, and style issues.
## Docs: https://staticcheck.dev/
staticcheck:
	@echo === staticcheck ===
	staticcheck ./...

## govulncheck: scan Go dependencies for known CVEs using call-graph analysis.
## Only reports vulnerabilities where the vulnerable function is actually called.
## Docs: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
govulncheck:
	@echo === govulncheck ===
	govulncheck ./...

## goimports: apply goimports formatting — gofmt + import grouping/ordering.
## Groups: stdlib, external, local module. Removes unused, adds missing imports.
## Docs: https://pkg.go.dev/golang.org/x/tools/cmd/goimports
goimports:
	@echo === goimports ===
	goimports -w .

## goimports-check: verify all files pass goimports (CI-friendly, no writes)
goimports-check:
	@echo === goimports (check) ===
	@goimports -l . && echo All files pass goimports.

## gofmt-check: verify all files pass gofmt with -s simplification (CI-friendly, no writes)
gofmt-check:
	@echo === gofmt (check) ===
	@gofmt -l -s . && echo All files pass gofmt.

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

# ─── Static Analysis (combined) ─────────────────────────────────────────────
# These targets orchestrate multiple tools for convenience.

## lint: quick lint (vet only, backward compatible alias)
lint:
	go vet $(PKGS)

## analyze: run ALL 9 static analysis tools sequentially, continue on errors.
## Use this for full project health check before committing.
analyze:
	@echo "============================================================"
	@echo " Static Analysis Suite - gitlab-mcp-server"
	@echo "============================================================"
	@echo ""
	@echo "[1/9] goimports (check)"
	-goimports -l .
	@echo ""
	@echo "[2/9] gofmt (check)"
	-gofmt -l -s .
	@echo ""
	@echo "[3/9] go vet"
	-go vet $(PKGS)
	@echo ""
	@echo "[4/9] modernize"
	-modernize ./...
	@echo ""
	@echo "[5/9] golangci-lint"
	-golangci-lint run ./...
	@echo ""
	@echo "[6/9] gosec"
	-gosec -severity medium -confidence medium -exclude-generated -fmt text ./...
	@echo ""
	@echo "[7/9] staticcheck"
	-staticcheck ./...
	@echo ""
	@echo "[8/9] govulncheck"
	-govulncheck ./...
	@echo ""
	@echo "[9/9] markdownlint"
	-npx markdownlint-cli2 "**/*.md" "#plan"
	@echo ""
	@echo "============================================================"
	@echo " Analysis complete. Review findings above."
	@echo "============================================================"

## analyze-fix: apply automatic fixes from format + lint tools.
## Order: goimports (formatting) → gofmt (formatting) → modernize (code) → markdownlint (docs).
## Always run 'make analyze' after to verify remaining findings.
analyze-fix:
	@echo === Applying automatic fixes ===
	@echo [1/4] goimports -w
	-goimports -w .
	@echo [2/4] gofmt -s -w
	gofmt -s -w .
	@echo [3/4] modernize -fix
	-modernize -fix ./...
	@echo [4/4] markdownlint --fix
	-npx markdownlint-cli2 --fix "**/*.md" "#plan"
	@echo === Fixes applied. Run 'make analyze' to verify. ===

## analyze-report: generate combined analysis report for LLM consumption.
## Output: dist/analysis/report.txt (Markdown-formatted, one section per tool).
analyze-report:
	$(call MKDIR_P,$(ANALYSIS_DIR))
	@echo "Generating analysis report to $(ANALYSIS_DIR)/report.txt ..."
	@echo "# Static Analysis Report - gitlab-mcp-server" > $(ANALYSIS_DIR)/report.txt
	@echo "# Tools: goimports, gofmt, go vet, modernize, golangci-lint, gosec, staticcheck, govulncheck, markdownlint" >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 1. goimports (check)" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-goimports -l . >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 2. gofmt (check)" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-gofmt -l -s . >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 3. go vet" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-go vet $(PKGS) >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 4. modernize" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-modernize ./... >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 5. golangci-lint" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-golangci-lint run ./... >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 6. gosec" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-gosec -severity medium -confidence medium -exclude-generated -fmt text ./... >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 7. staticcheck" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-staticcheck ./... >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 8. govulncheck" >> $(ANALYSIS_DIR)/report.txt
	@echo '```text' >> $(ANALYSIS_DIR)/report.txt
	-govulncheck ./... >> $(ANALYSIS_DIR)/report.txt 2>&1
	@echo '```' >> $(ANALYSIS_DIR)/report.txt
	@echo "" >> $(ANALYSIS_DIR)/report.txt
	@echo "## 9. markdownlint" >> $(ANALYSIS_DIR)/report.txt
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
	go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install golang.org/x/tools/cmd/goimports@latest
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

## docker-run: run the Docker image locally in HTTP mode (port 8080)
## Usage: make docker-run GITLAB_URL=https://gitlab.example.com
docker-run:
ifndef GITLAB_URL
	$(error GITLAB_URL is required. Usage: make docker-run GITLAB_URL=https://gitlab.example.com)
endif
	docker run --rm -p 8080:8080 \
		-e GITLAB_URL=$(GITLAB_URL) \
		$(BINARY_NAME):latest

# ─── Fly.io Deployment ───────────────────────────────────────────────────────
# Deploys the HTTP-mode server to Fly.io using fly.toml.
# Requires: flyctl (https://fly.io/docs/flyctl/install/) and `fly auth login`.
# The Docker image is built remotely by Fly's builder using the repo Dockerfile.

FLY_CONFIG ?= fly.toml
FLY_APP    := $(shell awk -F\" '/^app *=/ {print $$2; exit}' $(FLY_CONFIG) 2>/dev/null)

## fly-check: verify flyctl is installed and authenticated
fly-check:
	@command -v fly >/dev/null 2>&1 || { echo "flyctl not found. Install: https://fly.io/docs/flyctl/install/"; exit 1; }
	@fly auth whoami >/dev/null 2>&1 || { echo "Not authenticated. Run: fly auth login"; exit 1; }
	@echo "flyctl OK — app: $(FLY_APP)"

## fly-deploy: deploy current working tree to Fly.io (HTTP mode)
## Builds the image remotely with VERSION/COMMIT build args injected.
fly-deploy: fly-check
	fly deploy --config $(FLY_CONFIG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--image-label v$(VERSION)

## fly-deploy-release: deploy a tagged release to Fly.io.
## Verifies the working tree is clean and the current commit matches tag v$(VERSION).
## Use after `git tag vX.Y.Z && git push --tags` to ship a release.
fly-deploy-release: fly-check
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working tree is dirty. Commit or stash before deploying a release."; exit 1; \
	fi
	@expected="v$(VERSION)"; \
	tag_at_head=$$(git tag --points-at HEAD | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | head -1); \
	if [ "$$tag_at_head" != "$$expected" ]; then \
		echo "HEAD is not tagged $$expected (found: '$$tag_at_head'). Tag the release first."; exit 1; \
	fi
	@echo "Deploying release $(VERSION) to Fly app $(FLY_APP)…"
	fly deploy --config $(FLY_CONFIG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--image-label v$(VERSION) \
		--strategy rolling

## fly-status: show Fly.io app status (machines, regions, health)
fly-status: fly-check
	fly status --config $(FLY_CONFIG)

## fly-logs: tail Fly.io app logs
fly-logs: fly-check
	fly logs --config $(FLY_CONFIG)

## fly-ssh: open an SSH console to a running Fly.io machine
fly-ssh: fly-check
	fly ssh console --config $(FLY_CONFIG)

## fly-restart: restart all Fly.io machines (no redeploy)
fly-restart: fly-check
	fly machine restart --app $(FLY_APP)

# ─── LLM Discovery Files ─────────────────────────────────────────────────────

## gen-llms: generate llms.txt and llms-full.txt from registered tools/resources/prompts.
gen-llms:
	go run ./cmd/gen_llms/

## gen-readme: auto-generate meta-tool table in README.md from runtime tool definitions.
gen-readme:
	go run ./cmd/gen_readme/

# ─── Output Quality Audit ────────────────────────────────────────────────────

## audit-output: run MCP output quality audit on all tools.
## Checks: OutputSchema, Description "Returns:", Title field, Content annotations.
## Fails on regressions (non-zero findings).
audit-output:
	go run ./cmd/audit_output/

## audit-tokens: measure LLM context window overhead of all tool definitions.
## Reports per-tool token counts, domain totals, and mode comparison.
audit-tokens:
	go run ./cmd/audit_tokens/

# ─── Formatting ──────────────────────────────────────────────────────────────
# Prefer 'make goimports' over 'make fmt' — goimports is a superset of gofmt.

## fmt: apply gofmt formatting with -s simplification (legacy target)
fmt:
	gofmt -s -w .

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
# Requires: Node.js >= 22, npx, .env with GITLAB_URL and GITLAB_TOKEN.
# Compiles a fresh binary to /tmp, launches the Inspector, and cleans up on exit.

INSPECTOR_BIN := /tmp/$(BINARY_NAME)-inspector$(BINARY_EXT)

## inspector: compile the server and launch MCP Inspector UI via stdio.
## Reads credentials from .env. The temporary binary is removed on exit.
inspector:
	@if [ ! -f .env ]; then echo "ERROR: .env file not found. Create it with GITLAB_URL and GITLAB_TOKEN."; exit 1; fi
	@echo "Compiling $(BINARY_NAME) to $(INSPECTOR_BIN)..."
	@go build -ldflags="$(LDFLAGS)" -o $(INSPECTOR_BIN) $(CMD_PATH)
	@echo "Starting MCP Inspector (stdio) — press Ctrl+C to stop..."
	@trap 'rm -f $(INSPECTOR_BIN); echo "Cleaned up $(INSPECTOR_BIN)"' EXIT INT TERM && \
		set -a && . ./.env && set +a && \
		ALLOWED_ORIGINS="http://localhost:6274,http://127.0.0.1:6274,http://0.0.0.0:6274" \
		HOST=0.0.0.0 \
		npx -y @modelcontextprotocol/inspector \
			-e GITLAB_URL="$$GITLAB_URL" \
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
