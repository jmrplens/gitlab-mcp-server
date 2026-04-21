## Description

<!-- What does this PR do? Provide a clear summary of the change -->

## Related Issue

<!-- Link to the issue this PR addresses -->
Closes #

## Type of Change

<!-- Check all that apply -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Enhancement (improvement to existing functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Refactoring (no functional change)
- [ ] Documentation update

## Changes Made

<!-- List the key changes made in this PR -->

-
-

## How to Test

<!-- Steps for the reviewer to verify the changes -->

1.
2.
3.

## Checklist

### Code Quality

- [ ] Code compiles without errors: `go build ./...`
- [ ] Code passes lint: `go vet ./...`
- [ ] Code is formatted: `gofmt` and `goimports`
- [ ] No new warnings from `staticcheck`
- [ ] Follows idiomatic Go patterns

### Testing

- [ ] All existing tests pass: `go test ./... -count=1`
- [ ] New tests added for new functionality
- [ ] Test coverage >80% for modified packages
- [ ] Edge cases and error scenarios covered

### Documentation

- [ ] Doc comments added for exported types/functions
- [ ] `docs/` updated if public API changed
- [ ] `README.md` updated if user-facing behavior changed
- [ ] Commit messages follow conventional commits

### Security

- [ ] No secrets, tokens, or credentials in the code
- [ ] Input validation for user-provided parameters
- [ ] Error messages do not leak sensitive information

## Screenshots / Logs (if applicable)

<!-- Add screenshots, terminal output, or log snippets that help explain the change -->
