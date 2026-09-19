# Mutation and Condition Sweep

> **Diátaxis type**: How-to
> **Audience**: Maintainers and contributors closing the gate on one package
> **Prerequisites**: [Testing Documentation](README.md), section "Beyond Statement Coverage"

A sweep closes the mutation and condition gate on one Go package. This document
is the brief it is run from, and it is read in full before the first command,
because most of it is about reading what the two tools report rather than about
running them, and the last section is about the defects neither tool can report
at all.

The package is usually one of the 179 GitLab domain packages under
`internal/tools/`: typed input and output structs, handlers that call client-go,
markdown formatters registered in an `init()`, `ActionSpecs`, and table-driven
tests driving `testutil.NewTestClient` against an `httptest` server. Expect a
small package and a short run.

## Scope

One package, and nothing outside it. Do not run a repository generator
(`make gen-stats`, `make gen-testing-docs`, `make gen-site-stats`,
`make gen-llms`); a batch regenerates once at the end, because a generated
artifact refreshed per package conflicts with every other package in the same
batch.

A non-test file is edited only to fix a defect the sweep names, or to delete a
guard the survivor taxonomy says to delete, and the commit message says which
and why. Deleting an unobservable guard counts as naming a defect, so it needs
the same sentence as any other non-test edit. Restructuring that fixes nothing
belongs to a refactor and not here: a reviewer reading a test commit cannot tell a correction from a
rearrangement, and a batch that moved correct constants around in six packages
was filed as blocking on exactly those grounds.

## Commands

```bash
go test ./internal/tools/<pkg>/ -count=1
make coverage-mutants PKG=./internal/tools/<pkg>
make coverage-conditions PKG=./internal/tools/<pkg>
golangci-lint run ./internal/tools/<pkg>/...
```

Anything that writes a file runs where the tree is, `golangci-lint fmt` first
among them. A gate run on a remote builder measures a copy, and a formatter run
there formats the copy: the run reports clean while the committed file is not.

## The job

1. **Measure.** Run both gates and record the verbatim counts.
   `make coverage-mutants` prints `Killed: N, Lived: N, Not covered: N, Timed
   out: N`. `make coverage-conditions` (gobco) reports conditions never
   evaluated both ways; if it fails with a redeclaration panic the package has
   build-constrained files and gobco cannot read it, so say so and do mutation
   only. If the mutants gate refuses because the package does not pass its own
   tests, that refusal is the finding: report it and stop.

2. **Read the survivor list from a file, not from a streamed capture.** A sweep
   once found its captured output truncated at the head, four of six `LIVED`
   lines shown while the summary said six, with nothing saying the list was
   short. Redirect the run to a file and read that, then check the number of
   lines found against the `Lived:` and `Not covered:` counts the summary
   prints. If they disagree, the capture lied, not the tool.

3. **Verify every survivor by hand. Never trust the tool's label.** For each
   `LIVED` and each `NOT COVERED`, apply that exact mutation to the source, run
   the package's whole test suite, and see whether anything fails; then restore
   the file byte for byte and confirm with `git diff`. A `NOT COVERED` is a
   report about statement counters, not about the tests: many of them die when
   applied. A `TIMED OUT` is not a kill, and it may be an earlier test in the
   same binary hanging under the mutation and hiding the assertion that would
   have killed it.

   **Restore a probe with `git stash push -- <file>` or a patch file, never
   with `git checkout -- <file>`.** A checkout cannot distinguish the mutation
   from the fix written beside it, so it reverts both and the suite goes green
   because the defect is back. Commit the fix before probing where that is
   possible; nothing in a build can observe a working-tree operation that
   happened before it ran, so this one is prevented by procedure or not at all.

4. **Classify what really survives.** Read [Testing Documentation](README.md),
   section "Reading a survivor", which lists the six shapes no test can kill. A
   survivor matching one is recorded in the report with its file:line and its
   shape, and gets no test. Two of the six ask for more than a record. A guard
   that a second guard makes unobservable is deleted rather than recorded, since
   a copy nothing can observe is redundant code. An error branch that cannot
   fail for the type in hand is kept, because deleting it would discard a
   returned error and `errcheck` refuses that; write the test that pins whatever
   makes the condition unreachable instead, so the property behind the branch is
   held even though the branch is not. A survivor matching none of the six is a
   gap, and goes to the next step.

5. **Fix the real gaps.** Write a test that states the property the code has to
   hold rather than one repeating the value it currently has. Then prove it:
   re-apply the mutation, run the suite, see it fail, restore, confirm the suite
   passes.

   The gaps that recur in these packages are worth knowing before starting. A
   `!= nil` or `!= ""` guard around an optional field of a request body,
   inverted, means the caller's value never reaches GitLab while an unset one is
   sent as empty. A markdown formatter's branch for an empty list or a missing
   link goes unexercised. A pagination block is filled from the wrong response.
   An error-classification branch picks the wrong hint. A `related_actions`
   entry names an ID the catalog does not hold. Drive each through the handler
   with an `httptest` fixture rather than by calling the helper directly, so
   what is asserted is what a caller sees.

   Check the fixture spellings too: a fixture can name a JSON key the way
   GitLab's documentation reads rather than the way client-go decodes it
   (`external_url` against `externalUrl`), in which case the field arrives empty
   and every assertion about it passes vacuously. Verify against the pinned
   client-go source, not the documentation.

6. **Re-measure** and report the new counts.

## What done means

Either `Lived: 0` and `Not covered: 0`, or every remaining survivor named with
the shape it matches and the evidence gathered. A survivor nobody verified by
hand counts as neither.

## Classes neither gate can see

Both gates score branches of Go code: gremlins flips an operator and asks who
notices, gobco asks which operands were evaluated both ways. Everything below is
a defect neither question reaches, which is why each one has to be looked for by
hand during a sweep rather than waited for.

**A straight-line assignment is invisible to both.** Three packages swept early
reported `Lived: 0` and full condition coverage while a field was read from the
wrong source and nothing noticed, because a converter that assigns the wrong
neighbour has no branch to flip. Probe it deliberately: swap two assignments in
the converter, or drop one field from the request options, run the whole suite,
and see whether anything fails. Where it does not, the fixture is usually why,
since two fields carrying the same value are indistinguishable. Fix it with a
fixture in which no two values agree.

**A struct of booleans has no fixture where no two values agree.** Giving every
field a distinct value is impossible for a block of flags, and setting them all
to true is what hides a swap. Drive one flag at a time instead, which is also
what GitLab really answers, and compare the whole struct against one with only
that field populated. That distinguishes all of them.

**Canonical action IDs are unchecked by everything.** Every string in the
package that names a `domain.action` ID, whether it is `related_actions` in the
`ActionSpec` metadata, an `actionX = "domain.action"` constant `markdown.go`
builds its hints from, or an ID quoted in usage or parameter guidance, must name
an action the catalog really holds. A string literal is not a branch, so neither
gate can be wrong about it, and nothing else in the repository checks it either:
`audit_discovery_completeness` counts an *empty* `related`, never a wrong one.
Three packages have shipped IDs that answer a model "unknown action" the moment
it follows the hint. The group a package is aggregated into is declared in
`internal/tools/action_specs.go`, and `go run ./cmd/server --tool-search <term>`
prints the canonical IDs. Where a package keeps two sets of these constants, one
in `markdown.go` and one in `action_specs.go`, check that the two agree, since
they have drifted before; consolidate them **only where they disagree**, and
hold the result with a test.

**Prose that names another action names the canonical ID.** An `ActionSpec`'s
`Usage` text is served verbatim into the meta group description, so a sentence
there telling a model to call an individual tool name reaches a surface that
registers no such tool: the served catalog mentions 73 such names across 23
group tools today, measured over `internal/tools/testdata/tools_meta.json` by
taking every `gitlab_*` name its text carries and removing the 51 it registers. A canonical ID is the one form every surface resolves, the
dynamic surface by executing it and the meta and individual surfaces by
resolving it to their own spelling, which is why `toolutil.HintAction` composes
hints that way. No gate sees this because the text is data, not code, and a
model usually recovers, so the only evidence is a reader checking new text
against the surface that serves it. New text names the ID; correcting the tree
is a sweep of its own.

**A validation test that asserts only that an error came back proves nothing
about which layer refused.** The mutant dies and both gates go green, because
the test does notice the branch; what it does not notice is that the refusal
came from the mock rather than from the handler. Nothing static can separate the
two, and a rule demanding that every error be inspected would fire on 2977 of
the 3843 `if err == nil` assertions under `internal/tools/`. It is a review norm
instead: a test asserting that the handler refuses before it reaches GitLab uses
`testutil.ForbiddenHandler`, which fails the test if any request arrives, and a
test asserting an error GitLab produced asserts the error's content.

**A defect in a served string is a family defect, and the extent is greped
before it is reported.** This one is a rule about reporting rather than a
property of the tree, so there is nothing for a gate to read at all. The
evidence that it matters is a sweep that reported three dead hint IDs inside its
own 23 packages while the same class held 28 across 13 packages and roughly 188
related-action entries across 29, none of them swept. A report naming one
instance invites a one-line fix and leaves the rest. Where a gate for the class
exists, quote its count instead of judging the extent.

**A test's doc comment is not an assertion.** Two `projectimportexport` tests
claimed work they did not do, one saying it verifies that a handler sends the
description and upload fields while its own mock discarded the body, the other
saying it forwards an option it never read. Both gates are indifferent to
comments, so both passed, and a reader auditing coverage by reading test names
and comments would have marked the properties covered. When a comment claims a
property, check that the body asserts it, and correct the comment when it does
not.

## Conventions this repository enforces

- Change existing files with an editing tool that verifies the target text still
  matches. Do not edit with `sed`, `perl` or Python scripts. The one exception is
  applying and reverting a throwaway mutation probe, which must be verified
  reverted with `git diff`.
- A range over a case table that asserts must open one subtest per case with
  `t.Run`. `make check-test-subtests` gates this.
- Never call `t.Fatal` or `FailNow` off the test goroutine (`httptest` handlers,
  `go` statements): use `t.Errorf` plus a deterministic response and `return`.
- A test that reads `os.Stdout` or `os.Stderr` inside the test compares against a
  different file than the package bound at init, because CI runs under `go test
  -json`, which replaces both. Capture each side at the same moment.
- Test names are `TestThing_Scenario_ExpectedResult`, and a `_test.go` file
  exists only under the name of a module it tests.
- Every test added carries a doc comment saying what it asserts and why that
  matters, in the voice of the file around it. Match the surrounding comment
  length: the median comment block here is about three lines, and a block over
  eight is unusual.
- Run `golangci-lint run` on the package before committing.

## Committing and reporting

Commit with a conventional message (`test(<pkg>): ...` or
`refactor(<pkg>): ...`) whose subject says what the tests now hold rather than
that mutants were killed, under the configured global git identity, with no AI
attribution of any kind. If the package needed nothing at all, commit nothing and
say so.

Report the package, the before and after measurements verbatim, each real gap
with its file:line and the test that now kills it, each recorded survivor with
its shape, any defect found in non-test code, and the branch and commit so the
work can be integrated.

Verification by whoever holds one package does not substitute for verification
at integration: a sweep runs the gates it was told to run over the package it was
given, and cannot see a gate that reads the whole tree.
