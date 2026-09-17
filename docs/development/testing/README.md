# Testing Documentation

> **Diátaxis type**: Overview
> **Audience**: Users, evaluators, maintainers, contributors
> **Prerequisites**: Basic MCP concepts; Docker for live GitLab validation

This directory is the documentation hub for all validation work in
`gitlab-mcp-server`: conventional Go tests, real GitLab E2E tests, and AI
model evaluations that measure whether models can use the MCP catalog
correctly.

## Documents

| Document                                                             | Audience              | Purpose                                                                                                                |
| -------------------------------------------------------------------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| [Testing Reference](testing.md)                                      | Contributors          | Generated unit, integration, E2E, coverage, and package test reference.                                                |
| [E2E Coverage](e2e-coverage.md)                                      | Maintainers           | Generated per-runtime summary of what the end-to-end suite asserts, from the calls a Docker run recorded.              |
| [AI Model Evaluation](model-evaluation.md)                           | Users and evaluators  | Explains what AI model evaluations prove, how schema and Docker modes differ, and how to interpret the metrics.        |
| [AI Model Evaluation Developer Guide](model-evaluation-developer.md) | Maintainers           | Operational guide for running schema and Docker model evaluations, adding cases, reading traces, and updating results. |
| [AI Model Evaluation Results](model-results.md)                      | Users and maintainers | Current published benchmark result selected from generated reports.                                                    |

## Validation Layers

| Layer            | Runner                                         | GitLab backend                     | What it proves                                                                                                                                    |
| ---------------- | ---------------------------------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unit tests       | `go test ./internal/... ./cmd/...`             | Mock `httptest` servers            | Handler logic, schema validation, formatting, routing, and error handling.                                                                        |
| E2E tests        | `go test -tags e2e -p 1 ./test/e2e/gitlab/...` | Real GitLab, self-hosted or Docker | The MCP server can execute registered tools against GitLab APIs.                                                                                  |
| Model evaluation | `make modeleval-ce`, `make modeleval-ee`       | Docker GitLab CE or licensed EE    | A real model, given the surface a client is served, can pick the action and shape the arguments, and what it did to GitLab is checked afterwards. |

## Beyond Statement Coverage

Statement coverage is at or near 100% across `cmd/` and `internal/`, so it no
longer answers the question it is usually asked. Two tools answer what it
cannot, and both are run by hand on the package being worked on rather than
over the tree:

| Tool                                    | Command                                         | What it finds                                                                                           |
| --------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Condition coverage ([gobco][gobco])     | `make coverage-conditions PKG=./internal/oauth` | An operand of a compound condition that no test ever evaluated both ways: a covered line, half decided. |
| Mutation testing ([gremlins][gremlins]) | `make coverage-mutants PKG=./internal/oauth`    | A line every test runs and none asserts: the tool flips an operator or a boundary and asks who notices. |

[gobco]: https://github.com/rillig/gobco
[gremlins]: https://github.com/go-gremlins/gremlins

### What they cost, and why the figures are not generated

Measured on an 8-core machine, September 2026, one package at a time:

| Package                        | Own test time | gobco | gremlins |
| ------------------------------ | ------------: | ----: | -------: |
| `internal/config`              |         1.5 s |   4 s |    128 s |
| `internal/oauth`               |         0.2 s |   6 s |     88 s |
| `internal/gitlab`              |         8.0 s |  68 s |    229 s |
| `internal/serverpool`          |         1.0 s |  69 s |    207 s |
| `internal/tools/actioncatalog` |         0.5 s |  68 s |        — |
| `internal/subscriptions`       |         1.5 s |  72 s |        — |
| `internal/tools/dynamic`       |        35.0 s | 231 s |        — |
| `internal/tools`               |        89.2 s | 283 s |        — |

Four packages therefore cost about eleven minutes of mutation testing and
another two and a half of condition coverage, against roughly six minutes for
the whole-tree coverage pass `cmd/gen_testing_docs` runs. Extending either to
the packages that carry logic would be four to five times the generator's
current cost, which is why neither figure is a generated column in
[Testing Reference](testing.md): a number nobody can afford to regenerate goes
stale in the file while looking current.

`cmd/server` is absent from the table because it had never been measured: it
was assumed too slow to attempt. It is not. Measured on a 16-core machine
sharing the box with another run, its own tests take 30 s and a full gremlins
pass over its 1194 mutants takes 49 minutes. What makes that affordable is that
a killed mutant costs only as long as the first test that notices it, not a
whole suite run: 1194 mutants at 30 s each across four workers would be two and
a half hours, and the pass takes a third of that.

**gobco cannot analyse a package with build-constrained files at all.** It
copies every `.go` file into its work directory ignoring `//go:build`, so
`internal/toolutil` and `cmd/server` both fail with a redeclaration panic
(`openLeafNoFollow`, `bindUnixSocket`). Those two are covered by mutation
testing only.

**A gremlins run reporting TIMED OUT on a fast package has measured nothing.**
It derives each mutant's timeout from the package's own baseline times a
coefficient and applies no floor, so where the tests are quick that product
falls below the fixed cost of starting `go test` and every mutant is reported
timed out having never run. `internal/tools/surfaces`, whose tests take 0.015 s,
reported 1 killed and 12 timed out; given a budget that clears the startup cost
it reports 13 killed and none timed out. The reading is not merely incomplete,
it is flattering in both directions: a timeout is not a kill and gremlins leaves
it out of the efficacy quotient, so `internal/edition` announced 0.00% efficacy
over four mutants none of which ever ran, while `internal/telemetry` hid two
real survivors behind timeouts and read two better than it was.
`make coverage-mutants` therefore measures the package first and derives the
coefficient from it, printing both; `MUTANT_BUDGET` (30 s) is the floor.

**The coefficient is applied to gremlins' own coverage run, not to the baseline
the target printed.** gremlins multiplies `--timeout-coefficient` by however
long its own `go test -cover -coverprofile` took, and Go's test cache answers
that instantly for a package whose files have not changed since it last ran:
a second invocation on an unchanged tree, or the first after a `--dry-run`,
which gathers coverage the same way. Measured on `cmd/server`, whose tests take
44 s, the cached coverage run reported 0.65 s and derived a five-second budget
from it; the pass reported 0 killed and every mutant timed out, which reads
exactly like a package nothing tests. `make coverage-mutants` therefore runs
gremlins under `GOFLAGS=-count=1`, so what it multiplies is always a real
measurement. `go build` ignores a flag it does not know, so the same setting is
harmless for the compile around each mutant.

A timeout that survives a budget that size is a finding rather than a setting:
the mutant made the package pathologically slow instead of wrong. Eight of
`cmd/internal/mcpsurface`'s fourteen outlast 147 s, because what they mutate is
the memo in front of a catalog of some 1091 tools, so each lookup rebuilds it.
That the suite tolerates this says the tests benefit from the memo and none of
them asserts it, which is killed by a test that counts the builds, not by more
margin.

### Reading a survivor

Not every survivor is a gap. Three kinds cannot be killed by any test and
should be recorded rather than chased:

- **A boundary whose two sides agree at the boundary.** Flipping `>` to `>=`
  where both branches assign the same value at the boundary changes nothing.
  Six of the retry-clamp mutants in `internal/gitlab` are this shape.
- **A guard that a second guard makes unobservable.** The negative token
  cache used to check "disabled" in `Lookup` and `Contains` as well as in
  `RecordKind`, the only place an entry is stored, so removing either copy
  changed no answer. Such a copy is redundant code, and deleting it, as was
  done there, is a better answer than recording its survivors.
- **A tool artifact.** Mutations inside package-level constant initializers
  and `switch { case … }` expressions are reported as not covered because
  neither carries a statement counter, not because no test reaches them.

**Read the third kind, do not wave it through.** "Reached" is not "asserted",
and the tool that reports these can tell you neither. Of the 100 not-covered
mutants `cmd/server` reports, 26 are unkillable for reasons that are not about
the tests at all — 22 mutate a `+` between string literals into a `-`, which no
longer compiles, and four sit in Windows-only files or in a `testdata`
stand-in program the package never links. Of the remaining 74, twelve turned
out to be genuine gaps: every timeout constant was compared against itself
(`srv.ReadTimeout != baseHTTPReadTimeout`), so one collapsing to zero moved
both sides and failed nothing, and every refusal code was read off the wire and
compared with the same constant that wrote it, so a code that lost its sign
passed. All twelve were confirmed by hand — apply the mutation, run the whole
suite, see that nothing fails — and each now has a test that states the
property the constant has to hold rather than repeating its value.

## When To Use Each Layer

Use unit tests for implementation changes and regression coverage. Use E2E
tests when a handler or capability needs real GitLab behavior. Use schema model
evaluation when changing tool descriptions, meta-tool schemas, provider
adapters, or token budget. Use Docker model evaluation when the question is
whether a model can complete a realistic task through the actual MCP server and
a real GitLab API.

## Result Policy

A run writes observation and no verdict. Its shards land under `dist/modeleval/`,
which Git ignores, and every number a page shows is computed later, from those
shards and from the corpus at HEAD:

```bash
make model-results-record MODELEVAL_SHARDS=dist/modeleval/ce   # fold a run in and redraw
make model-results-refold MODELEVAL_SHARDS=dist/modeleval/ce   # re-score it under today's rules
make check-model-results                                        # the offline freshness gate
```

Two consequences worth stating before a run rather than after it. The shards are
the only thing a correction can be applied to, so **keep them**: the committed
record holds scored columns, a redraw scores nothing, and a run whose shards
were discarded can never be re-scored. And a fold refuses what it cannot
publish honestly, row by row and by name, including every row of a run made
with the fake provider, so folding a rehearsal publishes nothing and says why.

Do not hand-write a summary. What a page says comes from the record, and the
provenance a row carries (commit, date, instance edition and version, tier,
surface, mode, the corpus and contract digests, the tool-schema digest, and
what the run cost as four token figures) is what makes it a measurement rather
than a claim.

## Maintenance Rules

- When unit or E2E tests are added, modified, or removed, run
  `go run ./cmd/gen_testing_docs/` and keep [Testing Reference](testing.md)
  current.
- When model-evaluation cases, fixtures, or metrics change, update
  [AI Model Evaluation](model-evaluation.md),
  [AI Model Evaluation Developer Guide](model-evaluation-developer.md), and
  [AI Model Evaluation Results](model-results.md) together.
- Do not commit raw model traces, provider payloads, `.env` files, Docker
  fixture state, or generated reports under `dist/`.
