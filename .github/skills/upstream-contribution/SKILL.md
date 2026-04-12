---
name: upstream-contribution
description: "Contribute bug fixes or features to upstream projects (gitlab.com/gitlab-org/api/client-go). Use when: API gap found, client-go bug, missing endpoint wrapper."
---

# Upstream Contribution — GitLab client-go

Contribute bug fixes, missing endpoint wrappers, or features to the upstream GitLab API client library used by this project.

## Before Starting

1. Identify the specific gap or bug in `gitlab.com/gitlab-org/api/client-go/v2`
2. Verify it's not already fixed in the latest release
3. Check existing issues/MRs on the upstream project

## Upstream Project

- **Repository**: <https://gitlab.com/gitlab-org/api/client-go>
- **Language**: Go
- **Branch model**: `main` (default)
- **CI**: GitLab CI with Go tests
- **Style**: Standard Go conventions, `gofmt`, `golangci-lint`

## Steps

### 1. Fork and clone

```bash
# Fork via GitLab UI, then:
git clone https://gitlab.com/YOUR_USER/client-go.git
cd client-go
git remote add upstream https://gitlab.com/gitlab-org/api/client-go.git
git fetch upstream
```

### 2. Create feature branch

```bash
git checkout -b fix/endpoint-name upstream/main
```

### 3. Implement the fix

Follow existing patterns in the codebase:

- Services in individual files (e.g., `branches.go`, `issues.go`)
- Types match GitLab REST API v4 JSON responses
- Use `*int`, `*string`, `*bool` for optional fields in option structs
- Follow existing error handling patterns

### 4. Add tests

- Table-driven tests matching existing style
- Use `httptest` for mock API responses
- Test both success and error cases

### 5. Run CI checks locally

```bash
go test ./...
go vet ./...
golangci-lint run
```

### 6. Commit and push

```bash
git commit -m "feat: add EndpointName service method"
git push origin fix/endpoint-name
```

### 7. Create merge request

- Target: `main` branch on upstream
- Title: `feat: add EndpointName service method` (conventional commit style)
- Description: explain the use case, link to GitLab API docs
- Reference this MCP server project as the consumer

### 8. Update MCP server after merge

Once the upstream MR is merged and released:

```bash
go get gitlab.com/gitlab-org/api/client-go/v2@latest
go mod tidy
```

## Rules

- Match upstream code style exactly — do not introduce project-specific patterns
- Every new method must have tests with httptest mocks
- Optional fields use pointer types (`*string`, `*int`, `*bool`)
- Commit messages follow upstream conventions
- One logical change per MR — do not bundle unrelated fixes

## Validation Checklist

- [ ] Upstream tests pass: `go test ./...`
- [ ] Linting passes: `golangci-lint run`
- [ ] New code matches existing upstream patterns
- [ ] MR description explains the use case clearly
- [ ] GitLab API documentation link included in MR
