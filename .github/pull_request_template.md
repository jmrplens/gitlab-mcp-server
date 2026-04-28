## Description

<!-- What does this PR do? Provide a clear summary of the change -->

## Related Issue

<!-- Link to the issue this PR addresses (use "Closes #N" or "Refs #N") -->
Closes #

## Type of Change

<!-- Check all that apply -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Enhancement (improvement to existing functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Refactoring (no functional change)
- [ ] Documentation update
- [ ] CI / build / tooling

## Changes Made

<!-- List the key changes made in this PR -->

-
-

## How to Test

<!-- Steps for the reviewer to verify the changes -->

1.
2.
3.

## Breaking Changes / Migration Notes

<!-- If this is a breaking change, describe the migration path. Otherwise, write "N/A". -->

N/A

## Checklist

### Code Quality

- [ ] Code compiles: `go build ./...`
- [ ] `go vet` passes on changed packages
- [ ] Code is formatted: `make analyze-fix` (runs `goimports` + `gofmt`)
- [ ] `golangci-lint run` passes on changed packages (or `make analyze`)
- [ ] Follows idiomatic Go patterns and the conventions documented in `CLAUDE.md` / `.github/instructions/`

### Testing

- [ ] All existing tests pass: `go test ./... -count=1`
- [ ] New tests added for new functionality (table-driven, with `httptest` mocks)
- [ ] Coverage on modified packages is ≥ 90% (per project policy)
- [ ] Edge cases and error scenarios covered
- [ ] If applicable, E2E tests updated/added under `test/e2e/suite/`

### Documentation

- [ ] Doc comments added for exported types/functions (godoc)
- [ ] `docs/` updated if public API or behavior changed
- [ ] `README.md` / Starlight site (`site/src/content/docs/`) updated if user-facing behavior changed
- [ ] If introducing/removing a tool: `docs/tools/{domain}.md` and tool counts updated
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)

### Security

- [ ] No secrets, tokens, or credentials in code, tests, fixtures, or logs
- [ ] Input validation for user-provided parameters
- [ ] Error messages do not leak sensitive information
- [ ] No new dependencies with known vulnerabilities (verify with `govulncheck`)

## Screenshots / Logs (if applicable)

<!-- Add screenshots, terminal output, or log snippets that help explain the change -->
