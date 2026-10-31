// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command merge_coverage merges the Go statement coverage profiles of two
// measurements of the same packages, their offline tests and the runs against
// a real GitLab, and reports how much of each package the two reach together.
//
//	merge_coverage -packages <list> -offline <profile> [-offline <profile>]...
//	               -online <profile> [-online <profile>]... [-o <merged profile>]
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
// only the licensed run reaches was still reached.
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
// It exits 0 when it reported, 1 when a profile could not be read or merged, a
// package could not be measured or a file could not be written, and 2 for a
// command line it cannot use. make e2e-support-coverage runs the offline half
// and this command over the profiles the complete runs left.
package main
