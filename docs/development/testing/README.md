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

Running both over a package and answering what they report is a procedure this
project repeats. The brief a sweep of it is driven from is working material kept
outside the repository: it addresses one agent sweeping one package and names
paths on the maintainer's own machine. What is durable about the procedure is
written here instead, and this section is where a reader should start.

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

**gobco reads a package as every `.go` file in its directory, so the recipe
stages a package with build-constrained files.** gobco v1.3.4, the latest
release, hands the directory to `go/parser.ParseDir` and type-checks every
file together, ignoring `//go:build` lines and `_windows.go` names alike (its
default branch at 7a09995 does the same), so a function declared once per
platform is a redeclaration panic before anything is measured. Three packages
are in that shape: `cmd/server` (`peerStdinIsNull`, `bindUnixSocket` and
`isConnRefused`), `internal/toolutil` (`syncDirectory`, `openLeafNoFollow` and
`createNewLeafNoFollow`) and `test/e2e/internal/harness` (`serverBuildArgs`,
split on the race detector). Until issue 1017 the first two were covered by
mutation testing only.

`scripts/coverage-conditions.sh`, which `make coverage-conditions` calls,
copies the module into a temporary directory, leaving out `.git`,
`.claude/worktrees`, `node_modules` and `dist`, removes from the package's
copy every file the go command does not build here, runs gobco on the package
there, names the files it left out and removes the copy whatever happens. The
copy is of the module rather than of the package because gobco reaches an
external test package through the import path those tests already name: a
package copied under another path hands them the uninstrumented original, and
`internal/toolutil` failed to build that way. Tests also read `docs/`, the
root's `server.json` and their own directory's name, all of which a copy of
the module keeps. gobco also chooses which parsed files to instrument with a
context holding only `GOOS` and `GOARCH`. That context reads a constraint
naming only operating systems, architectures and `unix` exactly as the go
command does, and rejects one naming a tag, a release, cgo or the compiler,
which would compile the file and leave its conditions out of a report that
still reads complete. The constraint lines of those files, and only those, are
blanked line for line, so positions stay the original's, and a package holding
such a file is staged for that reason alone. A package with neither kind of
file runs where it is, a constraint on the platform alone included
(`internal/tools/packages`, whose `packages_stream_unix_test.go` sits behind
`//go:build !windows`), and `internal/config` still reports 424/424 in 3 s.
Measured on linux/amd64: `cmd/server` 1990/2032 (42 conditions left) and
`internal/toolutil` 3604/3782 (178 left).

**The platform halves the copy leaves out are not measured.** The Windows and
non-Linux halves of `cmd/server` (five files) and the non-Unix halves of
`internal/toolutil` (two files) build only on a platform other than the one
running gobco, and nothing runs gobco on Windows or macOS today. Their
conditions have never been measured, and they stay open under issue 1017,
as do the 42 and 178 conditions above that no test evaluates both ways.

**The e2e harness stays unmeasured until gobco honours build tags.**
`TAGS=<tag>` reaches `go list` and gobco's `go test` alike, and under
`TAGS=e2e` the harness compiles and its tests run. But every file of it sits
behind `e2e`, so every constraint line is blanked, and its own
`TestSources_EveryFile_CarriesExactlyTheE2EConstraint` fails on the blank
lines. The run fails there, and the script says the figure it printed
(863/1162) is not a measurement and names the blanked lines as the likely
cause. Without the blanking gobco declines every file and reports
`Condition coverage: 0/0`, which the script refuses: that is also what gobco
prints when no test wrote its counts, and it reads like a package with nothing
left to test.

**Nor a package whose `export_test.go` hands a symbol to its external test
package.** It resolves the `_test` package against the non-test files alone,
so `internal/tools/todos` dies with `action_specs_test.go:49:27: undefined:
todos.PublishedActionIDs` before instrumenting anything. Twenty-five of the
169 packages that carry an `action_specs.go` are in that state today. The
limitation is the tool's rather than the package's, which is why
`make check-spec-conditions` reports such a package as not measured instead
of failing on it.

**`PKG` names one package and gremlins reads it as a directory to walk.** It
mutates everything below the path it is given, so a package with anything under
it is measured together with its whole subtree, and a figure reported for the
parent is a figure about several packages. Measured on `./cmd/audit_1to1`: 789
runnable mutants, of which **27 are the package's own** and 762 belong to its
seven sub-packages, every one of which the sweep also measures separately. It
reaches fixtures the same way, so the planted trees under a command's
`testdata` were being mutated as though they were source.

`scripts/coverage-mutants.sh` therefore passes `--exclude-files=/` unless the
caller states an exclusion of their own. That flag is a regexp over the path
**relative to the target**, which is the one fact the rule rests on and was
measured rather than assumed: on that package `^internal/` and `/` both leave
24, while the module-relative `^cmd/audit_1to1/internal/` leaves all 789 and so
matches nothing. A path with a separator in it is exactly a file below the
package, and a leaf package has none, so the flag changes no figure there.

**A gremlins run over a `package main` directory reports every mutant killed
and has tested none of them, so the recipe stages such a package before
measuring it.** gremlins names the package to run the tests in by walking the
mutated file's directory upward until a component ends with the package
clause's name, and falls back to the module path when none does. For
`package main` in `cmd/audit_action_ids` nothing ends with "main", so it runs
`go test github.com/jmrplens/gitlab-mcp-server/v3` in its copy of the tree;
the module root holds no Go files, that fails at setup with exit 1, and exit 1
is what gremlins reads as KILLED. Measured on `cmd/audit_action_ids`: 134
killed, 0 lived, 3.5 seconds, against a suite that takes 9; the same tree
measured through a copy of the package in a directory whose name ends in
"main" reports 101 killed, 33 lived and 2 not covered in 10m49s. Every one of
the 40 `cmd/*` commands is `package main`, `cmd/server` included, so thirty
odd real survivors per command were being reported as kills. The giveaway is
on the face of such a run: a mutation pass finishing faster than one run of
the tests it claims to have run 134 times.

`scripts/coverage-mutants.sh`, which `make coverage-mutants` calls, therefore
copies a package whose directory does not end with its package name into a
sibling directory that does, runs gremlins against the copy, and removes it
through a trap. Measured on `cmd/audit_test_names`: 88 killed, 0 lived and
100% efficacy before, 83 killed, 16 lived and 3 not covered after. The copy
is a sibling rather than something tidier under `dist/` because Go's internal
rule is about the path, and a copy of a `cmd/` command staged elsewhere cannot
import `cmd/internal/...`. A staged copy that does not pass its own tests
where it was staged stops the run rather than falling back to the run that
lies, and a staged path that already exists is refused rather than removed.
A package whose directory already ends with its package name, which is all of
`internal/`, is measured where it is.

**A figure recorded for a `cmd/` command before this was fixed is not a
measurement.** The mutation numbers in the sweep that closed those packages
(PRs [#871](https://github.com/jmrplens/gitlab-mcp-server/pull/871) and
earlier) were produced by the unstaged run, so `Lived 0` from one of them says
nothing and the packages are owed a re-measurement. The coverage half was
valid throughout: NOT COVERED comes from `go test -cover` over the real
package, which gremlins runs correctly, so a package taken to `Not covered 0`
has been measured for reachability whatever the kill column said.

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
coefficient from it, printing both. `MUTANT_BUDGET` (300 s) is the budget, and
`MUTANT_BUDGET_FLOOR` (10 s) is what it may not go under: raising the budget is
the caller's business, and lowering it past a few seconds would recreate this
very defect, so a smaller value is raised to the floor and the run says so
rather than printing a budget it did not use.

**The coefficient is applied to gremlins' own coverage run, not to the baseline
the target printed.** gremlins multiplies `--timeout-coefficient` by however
long its own `go test -cover -coverprofile` took, and Go's test cache answers
that instantly for a package whose files have not changed since it last ran:
a second invocation on an unchanged tree, or the first after a `--dry-run`,
which gathers coverage the same way. Measured on `cmd/server`, whose tests take
44 s, the cached coverage run reported 0.65 s and derived a five-second budget
from it; the pass reported 0 killed and every mutant timed out, which reads
exactly like a package nothing tests. `make coverage-mutants` therefore runs
gremlins, and every go command before it, under `GOFLAGS=-count=1` (beside
`-trimpath`, for the reason below), so what it multiplies is always a real
measurement. A go command ignores a `GOFLAGS` entry it does not know, so the
same setting is harmless for `go list` and `go mod download`.

**The baseline is a run of the same command gremlins times, and it is timed by
the clock.** The recipe used to read the duration off the last line `go test`
printed, and to fall back to a guess of 0.010 s when that line carried none, a
guess meant for a cached result that `-count=1` had already made impossible.
What it read was wrong two ways, each measured. The tags:
`GREMLINS_FLAGS='--tags e2e'` reached gremlins and not the baseline, so on
`test/e2e/internal/harness`, where every file but `doc.go` carries
`//go:build e2e`, the baseline printed `[no test files]` and exited 0. The
guess gave a coefficient of 3001, gremlins' own coverage run took 114 s, and
every mutant got `go test -timeout 94h59m45s`. The INVERT_LOGICAL mutant of
`missing == 0 || time.Now().After(deadline)` in `awaitTraces` makes the span
wait loop for as long as a span is missing, and it ran for over an hour before
it was killed by hand; `test/e2e/internal/fixture` got 8 h 44 min per mutant
the same way. The units: a duration it could read was the test binary's own
run, without the build and link gremlins' wall clock includes, so
`internal/tools/elicitationtools` read 0.105 s against gremlins' 0.94 s and
gave each mutant about 269 s instead of the 30 s it printed. Nor could a parse
of that line be kept for the new command: the run gremlins times is a `-cover`
run, whose summary ends in a coverage figure rather than a duration.

`scripts/coverage-mutants.sh` therefore runs the command gremlins' coverage step
runs, `go test -count=1 [-tags T] [-coverpkg P] -cover -coverprofile F
./<pkg>/...` from the module root (`./...` under `--integration`). The tags and
the `-coverpkg` come from `GREMLINS_FLAGS`, read the way pflag reads it
(`-dte2e` included), or from gremlins' own `GREMLINS_UNLEASH_TAGS` and
`GREMLINS_UNLEASH_COVERPKG`, and a flag the script cannot read is refused,
since it could be hiding a tag. `--integration` is read from the flag alone
(`-i` in `GREMLINS_FLAGS`): gremlins v0.6.0 binds
`GREMLINS_UNLEASH_INTEGRATION` too, but reads it back with a bool type
assertion that the string an environment variable arrives as never passes, so
the variable widens nothing, and the script says so when it is set. The flag
is the only source of it the script can read: integration set in a
`.gremlins.yaml`, where a YAML bool does pass that assertion, reaches gremlins
and not the baseline, so ask for an integration run with `-i`. It runs
the command twice: once untimed, which is the pass/fail gate and leaves the
build cache as warm as gremlins' run will find it, and once under bash's
`time` in the C locale (under a comma-decimal locale `time` writes `0,940`,
which the script's positive-number check refuses, so every run there would
stop). Between the two it runs the command gremlins runs against each mutant,
once, for the reason the compile paragraph below gives. Measured after a
content edit, the timed base and gremlins' own figure now agree: 1.017 s
against 1.045 s on `elicitationtools`, 1.026 s against 1.038 s on
`cmd/audit_dynamic_aliases`. A
package with no test file under the tags it was given is refused, naming them,
since each mutant runs only that package's tests and none could be killed; the
exception is an integration run with a `-coverpkg` that names the package
(`GREMLINS_FLAGS='-i --coverpkg <pattern>'`), under which the module's other
tests cover the package and gremlins runs them against each mutant. The pattern
is resolved from the module root under the same tags, and one naming only other
packages is refused as well, since it leaves every mutant as uncovered as no
`-coverpkg` would, as is one `go list` cannot resolve at all. The exception
covers only an importable package measured where it is: a package main, or a
package measured through a staged copy (`<dir>.mutants-<name>`), is linked by
no other package's test, and gremlins matches the copy's files by their own
path, so either is refused whatever `-i` and `-coverpkg` say. A tag set only in a
`.gremlins.yaml` reaches gremlins and not the script, and that refusal stops
such a run only when it would find no test file at all: a package with some
untagged test files passes it and is measured against a baseline that runs
fewer tests than gremlins times. Pass the tag through `GREMLINS_FLAGS`.

`MUTANT_DEADLINE_MAX` (3600 s) holds each mutant's deadline to about its value,
applied through the coefficient because gremlins offers no other handle: a
ceiling below the floor is raised to it and the run says so, and a baseline
longer than half the ceiling is refused. About, because the cap is derived from
the script's baseline while gremlins multiplies the coefficient by its own
coverage run, which can come in a few percent longer. A mutant that hangs is
then reported TIMED OUT within about an hour rather than blocking a run for
days, and TIMED OUT is not part of the gate: it is a reading to explain, as the
paragraphs below do. The three knobs are seconds written as a plain number,
and anything else (`2h`) is refused rather than read as its leading digits.

A coefficient fixed in `GREMLINS_FLAGS` (`--timeout-coefficient 20`) is the one
gremlins applies, so it is the one the script passes and announces, with the
deadline it gives, and neither the budget nor the ceiling bounds it. The script
used to announce its own derived coefficient beside it, a deadline that was not
in force (on `internal/config`, about 300 s per mutant printed while gremlins
used 20). A value the script cannot read the way pflag and gremlins do is
refused before the suite runs: pflag reads a leading zero as octal, and gremlins
reads `0` as its own default of 3, which is what the script announces for it.

**A mutant's deadline pays for its run, not for a compile of the module.**
gremlins copies the module into a directory per worker and runs each mutant's
tests in that copy, and while `-trimpath` is off Go puts a package's directory
into its compile key (`buildActionID` in `cmd/go/internal/work/exec.go`, Go
1.27.1). Nothing compiled in the original tree was then of use to a worker, so
the first mutant on each of them recompiled every package of this module its
test imports, inside its own deadline. The inflated deadlines of the parsed
baseline absorbed that without anyone noticing, and a timed baseline did not:
`cmd/audit_dynamic_aliases` runs its tests in half a second and imports all of
`internal/tools`, four workers compiling it at once took 110 s each on five
cores and 2 s for every run after, and at a 30 s budget it reported 2 killed
and 8 timed out. The 300 s default budget was chosen to absorb that compile. A
run held to a fixed coefficient (`GREMLINS_FLAGS='--workers 2
--timeout-coefficient 20'`, which overrides the derived one) was not protected
by it: in issue 971, `issues`, `mergerequests`, `pipelineschedules`,
`repositorysubmodules`, `mrcontextcommits` and `cmd/audit_1to1/internal/sdk`
reported between two and four timeouts each, every one among the first mutants
a worker compiled.

`scripts/coverage-mutants.sh` therefore exports `GOFLAGS=-trimpath` (with
`-count=1`) to every go command it runs and to gremlins, which leaves the
directory out of the key, and runs the command gremlins runs against each
mutant (`go test -failfast [-tags T] <package>`, or `./...` under
`--integration`) once against the unmutated package, between the gate and the
timed run. The coverage runs cannot leave that behind by themselves: they
compile the packages their pattern or `-coverpkg` names with coverage counters,
which a mutant's plain `go test` does not share. That run is a gate as well,
since gremlins reads a failing run of that command as a killed mutant and a test
can pass under `-cover` and fail without it. A caller who states their own
`--exclude-files` may leave packages below the target to be mutated, each run
as its own `go test <package>`, so the run then covers the target's whole
subtree (`./<pkg>/...`). Under `--integration` it is a whole-module
`go test ./...`, untimed, on top of the two coverage runs of the whole module
the baseline already makes: the module's unit suite took 470 s on three cores
under `-trimpath` (below), so an integration run spends about that much more
before gremlins starts. Issue 1029 measured it on three
cores of a Linux build host (the `golang:1.27.1` image), on
`internal/tools/mrcontextcommits`:

| What                                                               | Without `-trimpath`          | With `-trimpath` and the warm-up run |
| ------------------------------------------------------------------ | ---------------------------- | ------------------------------------ |
| First mutant in a fresh copy of the module (two copies)            | 190 compiles, 28.4 s, 27.7 s | 1 compile, 1.36 s, 1.40 s            |
| `--workers 2 --timeout-coefficient 20`: killed / lived / timed out | 23 / 0 / 4                   | 27 / 0 / 0                           |
| the same: gremlins' own run, and the script end to end             | 92 s, 98 s                   | 30 s, 41 s                           |
| `--workers 2`, derived coefficient: killed / lived / timed out     | 27 / 0 / 0                   | 27 / 0 / 0                           |
| the same: gremlins' own run, and the script end to end             | 84 s, 89 s                   | 15 s, 19 s                           |

The four timeouts were the first four mutants to finish, and nothing else
moved: every figure after is the one before with the timeouts read as the
kills they are, as the issue's own six packages showed (27, 59, 112, 206, 318
and 49 killed). Under the derived coefficient no verdict moved at all, and the
run took a fifth of the time. The 300 s default stays: it is no longer paying
for a compile, but it is still what a slow mutant is given, and lowering it is
a separate decision. A timeout figure taken before issue 915 is not comparable
with one taken after it, since the deadlines themselves moved; one taken
before issue 1029 under a fixed coefficient counts first compiles as timeouts.

**`-trimpath` changes what two kinds of test see, and both were measured.** A
binary built under it records neither the directories of its sources nor the
root the toolchain is installed at. The second is where `runtime.GOROOT`, and
`go/build`'s `Default.GOROOT` after it, fall back when `GOROOT` is not set, and
a test binary starts with the environment the go command was given, so
`cmd/internal/golist`, `cmd/godoc_tool` and `cmd/gen_testing_docs`, which run
the go command out of that root, exec'd `bin/go` and failed. The script exports
`GOROOT` as `go env GOROOT` names it, which is the value an untrimmed build
records. The first has no such variable: `cmd/audit_metrics` found the
repository two directories above the path `runtime.Caller` reported for its own
source, which under `-trimpath` is the module-relative
`github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_metrics/main.go`, and now
walks up from its working directory to the `go.mod` instead. With both, the
whole module's unit suite passes under `-trimpath` (274 packages, 10 without
tests, 470 s on three cores), and so do `test/e2e/internal/fixture` and
`test/e2e/internal/harness` under `-tags e2e`. A test that reads its own
compiled source path again would fail the script's gate rather than be
measured, and the refusal names `-trimpath` as a cause to check, with the
command that reproduces the run: the `GOROOT`, `GOFLAGS` and tags it ran
under, over the package where it is rather than a staged copy that is gone by
then.

A timeout that survives the 300 s budget is a finding rather than a setting:
the mutant made the package pathologically slow instead of wrong. Eight of
`cmd/internal/mcpsurface`'s fourteen outlasted 147 s under the deadlines in use
before issue 915, because what they mutate is
the memo in front of a catalog of some 1091 tools, so each lookup rebuilds it.
That the suite tolerates this says the tests benefit from the memo and none of
them asserts it, which is killed by a test that counts the builds, not by more
margin.

**A timeout can also be a hang, and a hang hides every assertion behind it.**
`internal/gitlab` reported two, both in `limitedBody.Read`, and the assertion
that kills them was already written: `TestLimitedBody_OffersOneByteBeyondWhatIsLeft`
checks the size the inner reader is offered, and its own comment describes the
exact failure — a window that shrinks to zero makes a reader answer "no bytes,
no error" for ever. The tool never reached it. `capture_test.go` sorts before
`response_limit_test.go`, it is the first test in the package to drain a body
through the limiter, it drains with `io.ReadAll`, which takes no context, and
so it spun until the `go test` timeout and took the rest of the binary with it
unrun. Bounding that one drain — run the round trip on a goroutine, report the
stall, keep the assertion on the test goroutine — turns both from a ten-minute
timeout into a 5.3-second kill and costs nothing when the code is right. The
general shape is worth remembering: a survivor or a timeout is a claim about
the whole binary, so when the assertion you expected to kill it exists, check
whether anything earlier in the run stops the binary from getting there.

### Reading a survivor

Not every survivor is a gap. Six kinds cannot be killed by any test: four are
recorded and left alone, one is deleted rather than recorded, and one is kept
with the property behind it asserted instead. Read all six: the ones a sweep
most often meets are not the first ones listed.

- **A boundary whose two sides agree at the boundary.** Flipping `>` to `>=`
  where both branches assign the same value at the boundary changes nothing.
  Seven in `internal/gitlab` were this shape until issue 952 rewrote them
  with `min` and `max`, the answer the end of this bullet gives: five retry
  clamps in `retry.go` (`reset > wait`, both `wait > b.ceiling`,
  `wait < minWait` and `rateLimitResetWait`'s `wait < 0`; a sixth was the
  `reset <= 0` guard that the bullet on a guard made unobservable describes
  deleting), `limitedBody.Read`'s `int64(len(p)) > b.remaining+1`, where a
  buffer of exactly that length is cut to the length it already has, and
  `tierFromNamespaces`' `tier > best`, where a second namespace on the tier
  already held assigned `best` the value it had and changed nothing but
  whether one more debug line was written. So are three of `cmd/internal/apidocs`' four:
  `if secs <= 0 { return 0 }` is followed by `return time.Duration(secs) * time.Second`, `if d := time.Until(t);
  d > 0 { return d }` falls through to `return 0`, and `sleepCtx`'s
  `if d <= 0 { return ctx.Err() }` falls through to a zero timer that fires at
  once — at zero, each pair returns the same thing just as promptly.
  Where the branch does nothing but cut a value down to the bound, the `min`
  or `max` builtin says the same thing with no comparison left to flip, and
  that is the better answer: `internal/config`'s `announcedKeys` cut its list
  of key names behind `len(keys) > maxAnnouncedKeys`, which keeps the same ten
  keys either way when there are exactly ten, and now cuts it with `min`.
- **A tie-break comparator under an inequality guard.** The `sort.Slice` idiom
  `if a.x != b.x { return a.x < b.x }` has proved its two operands unequal
  before it compares them, so `<` and `<=` decide the same order and the
  boundary is unreachable by construction. All four survivors in
  `cmd/audit_1to1/internal/sdk` are this shape: three under an explicit guard,
  and the last level under the dedup above it, which is what makes a tie
  impossible there.
- **A clock boundary no test can schedule.** `time.Since(modTime) >= maxAge`
  and `> maxAge` differ only when the age is exactly `maxAge` to the
  nanosecond. A test cannot arrange that: the only input is a file's mtime, the
  filesystem clamps what is written to it (a pre-1901 time reads back as
  1901-12-13, so `time.Time.Sub` never reaches the saturation that would pin
  both sides to `math.MaxInt64`), and the wall clock moves between the write
  and the comparison. Killing it would mean indirecting the clock in production
  to assert a spelling. `cmd/internal/apidocs`' cache-freshness check is the
  one here. `internal/gitlab`'s initialization cooldown was the second until
  issue 952 tested it in a `testing/synctest` bubble: there both ends of the
  comparison are the bubble's clock, which moves only when the test sleeps,
  so the test lands on the boundary to the nanosecond. A boundary whose other
  end is a time the program holds, rather than one the filesystem stamps, can
  be scheduled that way, and is no longer this kind.
- **A guard that a second guard makes unobservable.** The negative token
  cache used to check "disabled" in `Lookup` and `Contains` as well as in
  `RecordKind`, the only place an entry is stored, so removing either copy
  changed no answer. Such a copy is redundant code, and deleting it, as was
  done there, is a better answer than recording its survivors. The
  `RateLimit-Reset` parser in `internal/gitlab` was the second case: its
  `reset <= 0` check could not be observed through the deadline check below it,
  since a Unix time at or before 1970 is already in the past, and it is gone.
- **A tool artifact.** Mutations inside package-level constant initializers
  and `switch { case … }` expressions are reported as not covered because
  neither carries a statement counter, not because no test reaches them. One
  shape among them is equivalent rather than unobserved: a product with a zero
  operand, such as `0 * time.Second`, is zero whatever the operator, so its
  mutant cannot be killed by any test. Spell the zero without the product
  (`time.Duration(0)`), as `internal/config`'s `DefaultDrainDelay` now is.
- **An error branch that cannot fail for the type in hand.** `json.Marshal` of
  a struct carrying no channel, function or cycle cannot return an error, and
  `json.Unmarshal` of that marshaller's own output into a `map[string]any`
  cannot either, so the four `err != nil` arms in `internal/tools/settings`
  (`settings.go:35`, `:40`, `:84` and `:89`) are never true whatever a test
  sends. Both halves of that were established rather than assumed: deleting all
  four leaves the suite green, and inverting them to `err == nil` fails four
  existing tests, so the arm that runs is exercised and the arm that returns is
  unreachable. What decides it is where the value came from, which is why the
  two arms above them, on the same round trip applied to the caller's own map
  (`:68` and `:73`), are reachable and have tests. This is not the guard the
  previous bullet describes: no second check is producing the same answer, the
  branch is the only reader of a returned error, and deleting it is what
  `errcheck` refuses. So the remedy differs too. Keep the check and assert the
  property that makes it unreachable, which here is that the round trip
  publishes what GitLab sent, key for key. The shape recurs wherever a package
  round-trips a struct through `encoding/json`, but it is a claim about the
  concrete type and never about the pattern: these four are reachable only
  through `*gl.Settings`, whose fields marshal and whose document is an object.
  A type that declares `MarshalJSON`, that can hold a `NaN` or an `Inf`, or
  that marshals to something other than an object can make the same branch
  fail, so name the type and say why its values cannot before filing one of
  these.

**Read the tool artifacts, do not wave them through.** "Reached" is not
"asserted", and the tool that reports these can tell you neither. Of the 100 not-covered
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

`cmd/internal/apidocs` is the same lesson at one tenth the size, and worth
naming because its ten not-covered mutants looked like nine artifacts and one
more. Six sit in a `switch { case … }` classifying an HTTP status, and all six
die when applied by hand. Four sit in the fetch-tuning constant block, three of
which die too. The tenth is `baseSpacing = 500 * time.Millisecond` mutated to
`500 / time.Millisecond`, which is zero, compiles, and survived: the only test
that read it expected `baseSpacing`, so the pause keeping a 250-page sweep under
GitLab's raw rate limiter could collapse to nothing with both sides of the
comparison moving together. The fix is the one `cmd/server`'s timeouts got — a
test that states what the constant has to be — and the general rule is that a
constant asserted only through the code that reads it is asserted by nothing.

### A test that reads `os.Stdout` or `os.Stderr` reads a different file under CI

CI runs the unit suite through `go test -json`, and in that mode the `testing`
package replaces `os.Stdout` and `os.Stderr` so it can attribute each line of
output to the test that wrote it. A value read inside a test is therefore a
different `*os.File` than the one a package bound at init, and an assertion
comparing the two passes under a plain `go test` and fails on all three
platforms in CI — the worst shape an assertion can have, because the local run
says nothing is wrong.

Capture both sides at the same moment instead: a package-level `var` in the
test file holds the shipped writer, and a second one holds `os.Stderr` taken
right beside it. `internal/cmdutil`'s `initialStderr` is the worked example,
and the test that needed it still fails when the package is pointed at
`os.Stdout`, which is the property it exists to hold.

### One condition class that gates

One narrow reading of gobco's output is a gate rather than a report, and it
came out of this sweep. `make check-spec-conditions`, which is
`scripts/check-spec-conditions.sh`, runs gobco over the `action_specs.go` of
the packages a branch changes and fails on a condition no test evaluates
both ways. The shape it is aimed at is the one the sweep keeps finding and
no other gate reads: a per-action metadata table plus a dispatch on the tool
name, where every entry of the table fills every field, so each
`if meta.x != ""` arm is always taken and its fallback can never publish
anything. The condition is then a decoration, and the mapping from action to
metadata is stated nowhere a reader can check.

It is narrow on purpose. gobco costs 60 to 80 seconds per package and caches
nothing, so the 169 packages carrying an `action_specs.go` would be about
three hours; a condition anywhere else in a package stays the sweep's subject
and is read with `make coverage-conditions`. It runs gobco through the same
`scripts/coverage-conditions.sh`, so a package with a file per platform is
staged and measured rather than reported as not measured. A condition that
genuinely cannot take the other value is declared on its own line, and a
declaration that answers nothing fails:

```go
if meta.usage != "" { // gobco: always true because every entry sets it
```

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
