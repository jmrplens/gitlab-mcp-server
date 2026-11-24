// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package recipe is how the coverage recipes run the programs they drive, and
// how a signal ends them.
//
// cmd/coverage_recipes mutates and measures a package through gremlins and
// gobco, and cmd/internal/conditions, which it and cmd/measure_conditions
// call, stages a copy of the module to measure one. Both were shell scripts
// (issue 1114, item 4), and both leave something on disk while a program
// runs: a copy of a package inside the module, or of the module in the
// temporary directory. So a signal may not end the process where it stands,
// and a handler that only cleaned up would let the run go on against the copy
// it had just removed, which is what the mutation recipe once did.
//
// What they did instead is what a shell does with a trap that exits, and it
// is what this package keeps. A program is started and waited for whatever
// happens ([Exec]): a signal the terminal sends reaches it directly, since it
// is in the same process group, and a signal sent to the recipe alone lets it
// finish, since a copy removed under a running program is worse than a wait.
// The recipe asks [Status] once it has ended, starts nothing more, removes
// what it staged, and exits with the status a shell gives the signal.
//
// A program is handed the environment the recipe was given, its locale
// included. The scripts exported LC_ALL=C to everything they ran, for their own
// awk, which reads a decimal point by the locale, and glob, which sorts by it.
// Nothing here reads a locale, so the tests of a measured package run under the
// one a plain go test would run them under.
//
// The programs are reached through a [Runner], so a test answers for the go
// command, gobco and gremlins without starting any.
package recipe
