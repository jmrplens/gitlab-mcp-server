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
- [ ] Consolidated Go analysis passes on changed packages (or `make golangci-lint` / `make analyze`)
- [ ] Code is formatted: `make analyze-fix` (runs configured `golangci-lint` formatters: `goimports`, `gofumpt`, and `gci`)
- [ ] Follows idiomatic Go patterns and the conventions documented in `CLAUDE.md` / `.github/instructions/`

### Testing

- [ ] All existing tests pass: `go test ./... -count=1`
- [ ] New tests added for new functionality (table-driven, with `httptest` mocks)
- [ ] Total coverage stays at or above the 90% CI enforces (`COVERAGE_MIN`), and the packages you touched are covered (the `increase-test-coverage` skill aims at 100% per touched package)
- [ ] Edge cases and error scenarios covered
- [ ] If applicable, E2E tests updated/added under `test/e2e/gitlab/`

### Documentation

- [ ] Doc comments added for exported types/functions (godoc)
- [ ] User-facing behavior changed: the site page that describes it updated in English (`site/src/content/docs/`) and Spanish (`site/src/content/docs/es/`) together
- [ ] Contributor-facing change (a gate, a generator, a convention): `docs/development/` or `CLAUDE.md` updated
- [ ] If introducing/removing a tool: tool reference regenerated (`make gen-tool-reference`), the site's counts refreshed (`make gen-site-stats`) and the fine-grained table regenerated (`make gen-action-grants`); generated pages are never edited by hand (`make update-all` runs every generator that needs no measurement, GitLab instance or paid run)
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)

### Security

- [ ] No secrets, tokens, or credentials in code, tests, fixtures, or logs
- [ ] Input validation for user-provided parameters
- [ ] Error messages do not leak sensitive information
- [ ] No new dependencies with known vulnerabilities (verify with `govulncheck`)

## Screenshots / Logs (if applicable)

<!-- Add screenshots, terminal output, or log snippets that help explain the change -->
