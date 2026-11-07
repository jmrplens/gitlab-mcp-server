// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command merge_coverage merges the Go statement coverage profiles of two
// measurements of the same packages, their offline tests and the runs against
// a real GitLab, and reports how much of each package the two reach together.
// It also writes the committed record of what the runs reached, and holds the
// offline half of a later tree to that record, which is the gate CI runs.
//
//	merge_coverage -packages <list> -offline <profile> [-offline <profile>]...
//	               -online <profile> [-online <profile>]... [-o <merged profile>]
//	               [-write-record <record>] [-summary <markdown file>]
//	merge_coverage -packages <list> -offline <profile> [-offline <profile>]...
//	               -check-record <record> [-freshness checked|deferred]
//	               [-summary <markdown file>]
//
// It exists for the support packages of the end-to-end suite,
// test/e2e/internal/fixture and test/e2e/modeleval (issue 1461), whose code
// is mostly the part that only runs against a GitLab: the fixture builders
// and the sessions that start the real binary. Their offline tests reach the
// rest, so neither measurement alone says what the package's tests reach, and
// the two together do.
//
// # The two halves
//
// An -offline profile is one the packages' own tests wrote with no GitLab,
// `go test -tags e2e -coverpkg=<list> -coverprofile=<file> <packages>`. An
// -online profile is one a run against GitLab wrote, the test binaries of the
// rebuilt suite built with the same -coverpkg (test/e2e/scripts/run-docker-e2e.sh
// writes <stem>-coverprofile.out when E2E_COVERPKG names the packages). Each
// flag takes one file and may be given again, so a CE run, its final pass and
// a licensed run fold into one figure, which is the honest one: a statement
// only the licensed run reaches was still reached. A test binary writes its
// profile when its last test ends, inside m.Run, so what the harness runs
// after that, its AtExit hooks, is in no run profile.
//
// -packages is the list the profiles were written for, comma-separated as
// go test's -coverpkg takes it: a directory below the module root spelled
// ./dir, or an import path. It names packages, not patterns.
//
// # The merge
//
// Every profile is read with golang.org/x/tools/cover, which folds the
// blocks one profile repeats (go test writes a block once per test binary
// that measured it). Across profiles a block is the same block when it has
// the same file and the same extent, and it counts once, however many
// profiles carry it: a statement is reached when any profile reached it.
//
// Two profiles written from the same source agree on every block, so a
// disagreement is refused rather than merged: the same extent counting a
// different number of statements, or two blocks of one file that overlap
// without being the same block. Either means the profiles describe two
// versions of a file, and a figure folded from both would be about neither.
//
// The merged profile keeps the mode of its inputs when they share one, with
// the counts of a counting mode added; inputs of different modes merge in set
// mode.
//
// # The report
//
// One row per package of -packages: what the offline tests reached, what the
// runs reached, what the two reach together, and how many statements only
// the runs reached. A half none of whose profiles carries a block of the
// package reads absent: no test binary of that half imports it, or the half
// did not name it. A package no profile carries at all is refused, since
// there is nothing to measure it by. A percentage is truncated rather than
// rounded, so a package short of its last statement never reads 100.0%.
//
// The report is printed, and with -summary appended to the file named, which
// is how a workflow puts it on its run ($GITHUB_STEP_SUMMARY). -o writes the
// merged profile, which go tool cover reads like any other.
//
// # The record
//
// The complete runs need a GitLab, so CI cannot take their half; the record
// is that half, committed (docs/development/e2e-support-coverage.json).
// -write-record writes it beside the report: the day it was taken in UTC,
// the run profiles it was folded from, each package with its statements,
// whether any run profile carried it, and how many of its statements neither
// half reached, and every top-level declaration the runs reached into, keyed
// by kind and name (func (*Builder) Project, var b), with the SHA-256 of its
// text and each block they reached, its lines counted from the
// declaration's first line. The doc comment is outside the hashed text and
// the lines are relative, so its doc comment reworded, or a comment or code
// changed elsewhere in the file, leaves a declaration standing, and an edit
// of its own text, a comment inside it included, drops it: the runs ran the
// old code, and what they reached in it says nothing about the new.
//
// # The gate
//
// -check-record reads the runs' half from a record instead of from -online
// profiles. Each recorded declaration whose text is still what the runs ran
// is credited with the blocks they reached in it; one whose text changed,
// that its file no longer declares, whose file the offline half no longer
// measures, or whose recorded blocks are not the ones go test instruments in
// it today (another toolchain's instrumentation), is dropped whole and named.
// Each package is then held to the record: more statements reached by
// neither half than the record says fails, naming the two ways out, a test
// that reaches them or a complete run recorded again. With -freshness
// deferred, which ci.yml passes below a stack's top, such an excess is a
// note instead when the record predates the package, because a declaration
// of it was dropped or it counts other statements than the record did, since
// the stack's top records a complete run of its own tree; an excess with the
// record still describing the package, which is an offline test lost, fails
// either way. Fewer statements than the record allows is a note that the
// next record lowers it. A record that does not exist yet is the state
// before the first complete run is recorded, and is reported as such, holding
// nothing; one that cannot be read, is another schema, or was taken of other
// packages, is refused.
//
// It exits 0 when it reported, or the gate passed, 1 when a profile or the
// record could not be read or merged, a package could not be measured, the
// report could not be printed, a file could not be written or the gate
// failed, and 2 for a command line it cannot use. make e2e-support-coverage runs the offline half and this command over
// the profiles the complete runs left, make e2e-support-coverage-record does
// the same and writes the record, and make check-e2e-support-coverage is the
// gate.
package main
