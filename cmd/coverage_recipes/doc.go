// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command coverage_recipes runs the two tools that measure what a package's
// tests decide, gobco over its conditions and gremlins over its mutants, the
// way this repository needs them run, and the gate that reads gobco's report
// over action specs:
//
//	coverage_recipes conditions <package> [tags]
//	coverage_recipes install-gobco
//	coverage_recipes mutants <package> [budget] [floor] [ceiling]
//	coverage_recipes spec-conditions [base]
//
// They are the recipes behind `make coverage-conditions`, `make
// coverage-mutants` and `make check-spec-conditions`, and the first two what
// CI's cross-platform legs and conditions.yml run. Each was a shell script
// until issue 1114 (scripts/coverage-conditions.sh,
// scripts/coverage-mutants.sh and scripts/check-spec-conditions.sh), and every
// recorded run of the three is replayed by the tests.
//
// conditions and install-gobco are [conditions.Measure] and
// [conditions.Install], whose package says what gobco cannot read and how it
// is measured all the same.
//
// mutants makes gremlins' verdicts mean what they say. gremlins decides which
// package to run the tests in by walking up from the mutated file's directory
// until a path component ends with the package clause's name, and falls back
// to the module path when none does (internal/engine/engine.go, func pkgName,
// v0.6.0). For `package main` in a directory that is not called main, nothing
// matches: it runs `go test` on the module path, this module's root holds no
// Go files, that exits 1, and exit 1 is how gremlins recognizes a killed
// mutant. Every covered mutant of every command under cmd/ is then reported
// KILLED without its tests ever running.
//
// Measured on cmd/audit_action_ids: the ordinary run says 134 killed, 0 lived,
// in 3.5 seconds, against a suite that takes 9. The same tree measured through
// a copy of the package in a directory whose name ends in "main" says 101
// killed, 33 lived, 2 not covered, in 10m49s. The coverage half is honest
// either way, since NOT COVERED comes from a real `go test -cover` over the
// real package; it is the kill verdict that is fabricated, so `Lived 0` from
// an unstaged run of a command says nothing at all.
//
// So such a package is staged into a directory inside the module whose name
// ends in "main", gremlins is run there, and the copy is removed afterwards.
// The staged copy is a real package of this module, so the resolution lands on
// it and the tests that decide each verdict are the package's own. Where the
// copy goes is decided by Go's internal rule, and is told with the code that
// decides it. The copy holds the package's own files and every directory below
// it that is not a package (testdata, assets), and leaves out the packages
// below it, which the copy imports where they are: so the staged run measures
// the package that was named and nothing else.
//
// What this does NOT do is paper over a staged run that cannot work. If the
// copy does not pass its own tests where it was staged, the run stops and says
// so rather than falling back to the unstaged run, because that run is the one
// that reports a clean package without measuring it. See issue 872.
//
// The second thing it decides is how long each mutant may run. gremlins has no
// setting for that: it multiplies --timeout-coefficient by the wall time of
// its own coverage run, `go test [-tags T] [-coverpkg P] -cover -coverprofile
// F ./<pkg>/...` from the module root. So the coefficient is derived from a
// run of that same command under the same tags, timed by the clock, and held
// under a ceiling. Deriving it from anything else is how a package behind a
// build tag came to give each mutant a deadline of 95 hours. See issue 915.
//
// The third is what that deadline has to pay for. gremlins runs each mutant's
// tests in its own copy of the module, one per worker, and Go keys a compile
// on the package's directory unless -trimpath is set, so the first mutant on
// every worker used to recompile every package of this module its test imports
// inside that mutant's own deadline. Every go command here and in gremlins
// therefore runs under -trimpath, and the command gremlins runs per mutant is
// run once beforehand, so a worker's copy finds everything it needs already
// compiled. See issue 1029.
//
// A signal this process receives ends any of them with the status a shell
// gives it, once the program that is running has ended and what was staged is
// gone (cmd/internal/recipe). `go run` ends on a SIGTERM without passing it on,
// so the make targets build this command and run it in place of the shell make
// started, which is the process make passes a signal to; one started through
// `go run` is ended by a signal to its process group, or to itself.
package main
