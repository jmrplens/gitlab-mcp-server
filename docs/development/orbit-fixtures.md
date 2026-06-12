# Orbit live test fixtures

The Orbit handler live tests in `test/e2e/orbit/live_test.go`
exercise the real `https://gitlab.com/api/v4/orbit/*` endpoints against
the `plens1` group on gitlab.com. Those tests are gated behind the
`orbitlive` build tag and require `GITLAB_COM_TOKEN` in the environment.
This document explains the data they expect and how to re-provision
it.

## What the tests need

`TestOrbitLiveGitLabCom` and `TestOrbitLiveGitLabCom_ShapeDiscovery`
are self-contained: they query by `full_path` filter
(`full_path: "plens1/*"`) and use no hardcoded project ids. They will
return rows against any namespace that has at least:

- a `Project` matching `full_path: "<namespace>/kg-fixtures"`
- a `Project` matching `full_path: "<namespace>/security-fixtures"`
- at least one `Milestone` with `state: "active"`
- at least one `MergeRequest` with `source_branch: "feature/restock-helper"`
- at least one detected `Vulnerability`

`TestOrbitLiveGitLabCom_Fixtures` filters by content
(`path ends_with .py`, `state = detected`, etc.) so it works against
any namespace once the indexer has had time to scan the fixture
content.

## Reproducing the fixtures

```bash
# 1. Create a personal access token with api + write_repository scopes
# 2. Run the setup script with the namespace and token

GITLAB_COM_TOKEN=glpat-... \
ORBIT_FIXTURES_NAMESPACE=mynamespace \
./scripts/setup-orbit-fixtures.sh
```

The script is **idempotent**: it checks for existing resources via the
API and skips them. Re-running it on an already-provisioned namespace
is a no-op.

### What the script creates

| Project | Content | Why |
|---|---|---|
| `<ns>/kg-fixtures` | Python package `acme.orders` (7 source files, 2 test files) + `.gitlab-ci.yml` (4 stages, 5 jobs, SAST, Secret-Detection) + 7 labels + 1 milestone + 5 issues + 2 environments + 1 squash-merged MR | Populates File, Definition, Directory, ImportedSymbol, Branch, MergeRequest, MergeRequestDiff, MergeRequestDiffFile, Note, Label, Milestone, WorkItem, Pipeline, Job, Stage, Environment |
| `<ns>/security-fixtures` | Intentionally vulnerable code (hard-coded AWS keys, SQL injection, weak MD5 hashing, `eval()`) + 4 GitLab-managed security scanning templates | Populates Vulnerability, Finding, SecurityScan, VulnerabilityIdentifier, VulnerabilityOccurrence, VulnerabilityScanner |

The script provisions everything from `test/fixtures/orbit/{kg,security}-fixtures/`
in this repo. The fixture content is committed and reproducible —
no external state is needed.

## Optional: mirror a public GitLab repo

The setup script accepts `--mirror-cli` to additionally push
`gitlab-org/cli` (the glab CLI, ~10k files, 1k stars, 1000+ branches)
into `<ns>/glab-mirror`. The mirror takes several minutes and adds
real-world data: hundreds of MergeRequests, real CI runs on main,
real cross-file `ImportedSymbol` references.

```bash
GITLAB_COM_TOKEN=glpat-... \
ORBIT_FIXTURES_NAMESPACE=mynamespace \
./scripts/setup-orbit-fixtures.sh --mirror-cli
```

Re-runs are safe: the mirror step checks for project existence and
skips if already present.

## Indexer behavior

The Orbit indexer is eventually consistent. After a fresh push:

1. The indexer picks up the new content within 1–5 minutes.
2. `indexing.state` may transiently report `"error"` even while
   `last_completed_at` advances and row counts keep growing. This is
   a GitLab-side artifact, not a fixture issue.
3. The live test assertions are **informational** (`row_count > 0`)
   rather than strict equality, so a transient re-indexing blip
   surfaces as `PASS` with a low `row_count`, not a hard failure.

If `row_count` is consistently 0 long after the script completes, the
indexer has not yet linked the resource. Re-run the test after a
few minutes, or check `GET /api/v4/orbit/graph_status?full_path=<ns>`
to see what the indexer knows.

## Project layout

```
test/fixtures/orbit/
├── kg-fixtures/
│   ├── README.md
│   ├── pyproject.toml
│   ├── .gitlab-ci.yml
│   ├── src/acme/__init__.py
│   ├── src/acme/orders/__init__.py
│   ├── src/acme/orders/__main__.py
│   ├── src/acme/orders/cli.py
│   ├── src/acme/orders/fulfillment.py
│   ├── src/acme/orders/models.py
│   ├── src/acme/orders/pricing.py
│   └── tests/
│       ├── test_fulfillment.py
│       └── test_pricing.py
└── security-fixtures/
    ├── README.md
    ├── pyproject.toml
    ├── .gitlab-ci.yml
    ├── src/acme/__init__.py
    ├── src/acme/orders/__init__.py
    ├── src/acme/orders/auth.py
    ├── src/acme/orders/legacy_db.py
    └── tests/
        └── test_smoke.py
```

Both fixture sets are intentionally small (a few hundred lines each)
so the Orbit indexer can process them within a couple of minutes.

## Running the live tests

```bash
# Unit + integration tests (does not run the live tests, which need the orbitlive tag)
go test ./internal/tools/orbit/

# Live integration test (gated by build tag and token)
GITLAB_COM_TOKEN=glpat-... \
  go test -tags orbitlive -count=1 -v ./test/e2e/orbit/
```

Override the fixture namespace with `ORBIT_FIXTURES_NAMESPACE`:

```bash
GITLAB_COM_TOKEN=glpat-... \
ORBIT_FIXTURES_NAMESPACE=acme-research \
  go test -tags orbitlive -count=1 -v -run 'TestOrbitLiveGitLabCom_Fixtures' ./test/e2e/orbit/
```
