// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// What the mutation recipe's knobs are when the caller gives none. `make
// coverage-mutants` always passes the first three, as MUTANT_BUDGET,
// MUTANT_BUDGET_FLOOR and MUTANT_DEADLINE_MAX, so the Makefile's are the ones
// a run gets through it, and a test holds the two sets together.
const (
	defaultBudget    = "300"
	defaultFloor     = "10"
	defaultCeiling   = "3600"
	defaultMemoryMax = "8"
)

// largestMemoryMax is the largest ceiling on the address space a run may ask
// for, in GiB: 1 PiB, past which it bounds nothing a host can hold.
const largestMemoryMax = 1048576

// The bounds of a derived coefficient: never below the floor, so a slow
// package still gets a real multiple of its own runtime, and never above the
// largest.
const (
	coefficientFloor   = 8
	largestCoefficient = 6000
)

// seconds is a knob: a plain number of seconds. Anything else is refused,
// since a reader of numbers that takes the leading digits and drops the rest
// would read a ceiling written 2h as 2, raise it to the floor, and give every
// mutant about ten seconds, with a notice about the floor as the only sign of
// it.
var seconds = regexp.MustCompile(`^\d+([.]\d+)?$`)

// knobs are the bounds of one run: how long each mutant may take, the ceiling
// on it, both as they were written, so a message shows the caller's own
// figure, and the ceiling on the address space of what gremlins starts, in
// GiB, 0 for none.
type knobs struct {
	budget    string
	ceiling   string
	memoryMax uint64
}

// readKnobs reads the arguments after the package, and MUTANT_MEMORY_MAX from
// the environment, and returns what it has to say about them.
//
// The budget is a knob and the floor is what it may not go under: below a few
// seconds the per-mutant budget falls under the fixed cost of starting
// `go test` and every mutant is reported TIMED OUT having never run. The
// floor is applied out loud, or the printed budget is a second lie on top of
// that.
//
// The ceiling holds each mutant's deadline to about its value, so a mutant
// that makes the tests hang is reported TIMED OUT within about that long
// rather than holding a worker for as long as the coefficient allows. About,
// because the cap is derived from this recipe's baseline while gremlins
// multiplies the coefficient by its own coverage run: a run of that command a
// few percent slower than this one gives a deadline a few percent over the
// ceiling. It answers to the same floor as the budget, for the same reason:
// under it every mutant times out unrun.
func readKnobs(args, environ []string) (read knobs, notices []string, err error) {
	given := []string{defaultBudget, defaultFloor, defaultCeiling}
	for i, arg := range args {
		if arg != "" {
			given[i] = arg
		}
	}
	budget, floor, ceiling := given[0], given[1], given[2]
	for _, knob := range []struct{ name, value string }{{"MUTANT_BUDGET", budget}, {"MUTANT_BUDGET_FLOOR", floor}, {"MUTANT_DEADLINE_MAX", ceiling}} {
		if !seconds.MatchString(knob.value) {
			return knobs{}, nil, fmt.Errorf("%s=%s is not a number of seconds; MUTANT_BUDGET, MUTANT_BUDGET_FLOOR and MUTANT_DEADLINE_MAX (the recipe's second, third and fourth arguments) are all written as seconds, such as 300 or 3600", knob.name, knob.value)
		}
	}
	for _, bound := range []struct {
		name  string
		value *string
	}{{"MUTANT_BUDGET", &budget}, {"MUTANT_DEADLINE_MAX", &ceiling}} {
		if number(*bound.value) < number(floor) {
			notices = append(notices, fmt.Sprintf("%s=%ss is under the %ss floor and would report untested mutants as timeouts; using %ss", bound.name, *bound.value, floor, floor))
			*bound.value = floor
		}
	}
	memoryMax, err := readMemoryMax(environ)
	return knobs{budget: budget, ceiling: ceiling, memoryMax: memoryMax}, notices, err
}

// readMemoryMax reads the ceiling on the address space of gremlins and what it
// starts, in GiB.
//
// The deadline bounds how long a mutant runs, not how much it holds, and a
// mutant can turn a loop that ends into one that never does and allocates on
// every pass. Such a test binary grows by about 1 GiB a second: `index + 1` ->
// `index - 1` in makeArgument.take (cmd/audit_supply_chain) re-reads one make
// option for ever, and `i++` -> `i--` in beforeVerb (cmd/audit_tenancy) writes
// one byte for ever. On 2026-11-12 test binaries of those two packages reached
// up to 24.1 GiB (cmd/audit_supply_chain) and 28.5 GiB (cmd/audit_tenancy) on
// the build host before the kernel's OOM killer stopped them, together with an
// unrelated process. So gremlins, and every go command and test binary it
// starts, runs under an address-space ceiling of MUTANT_MEMORY_MAX GiB
// (RLIMIT_AS): a runaway ends in `fatal error: out of memory` within seconds,
// its test fails, and gremlins reports it KILLED, the verdict any failing test
// gives a mutant. Address space is not resident memory, since the Go runtime
// reserves ahead of what it touches: under 3 GiB the make.go runaway stopped
// at 660 MiB resident. The heaviest suites measured (cmd/audit_readonly_graphql,
// cmd/server, internal/tools) pass under the default. The ceiling is per
// process, so gremlins' four workers can hold four times it between them. 0
// sets no ceiling, a tighter limit the run inherited is kept rather than
// raised to this one, and outside Linux, the kernel this relies on to enforce
// the limit, the run goes on without one and says so.
//
// The value is a whole number written without a leading zero, as a fixed
// coefficient is, so a figure reads the same to whoever wrote it, and one past
// 1 PiB is refused, since it bounds nothing a host can hold.
func readMemoryMax(environ []string) (uint64, error) {
	given, _ := recipe.Getenv(environ, "MUTANT_MEMORY_MAX")
	if given == "" {
		given = defaultMemoryMax
	}
	if !wholeNumber.MatchString(given) {
		return 0, fmt.Errorf("MUTANT_MEMORY_MAX=%s is not a whole number of GiB written without a leading zero; write it as 8, or as 0 for no ceiling", given)
	}
	gib, err := strconv.ParseUint(given, 10, 64)
	if err != nil || gib > largestMemoryMax {
		return 0, fmt.Errorf("MUTANT_MEMORY_MAX=%s GiB is more than %d (1 PiB), so it bounds nothing; write 0 for no ceiling", given, largestMemoryMax)
	}
	return gib, nil
}

// number is the value of a figure [seconds] admits, which always parses.
func number(figure string) float64 {
	value, _ := strconv.ParseFloat(figure, 64)
	return value
}

// deadline is how long each mutant is given: the coefficient gremlins is
// handed, what is said about it, and whether gremlins is handed one at all.
type deadline struct {
	coefficient string
	notices     []string
	derived     bool
}

// deriveDeadline derives the coefficient from base, the baseline's seconds as
// they are printed.
//
// It is the budget's multiple of the base, never below 8 so a slow package
// still gets a real multiple of its own runtime and never above 6000, and then
// no larger than the ceiling's multiple. A ceiling that does not hold two runs
// would time out every mutant, so it is refused rather than applied. gremlins
// multiplies its OWN coverage run rather than this one, and Go's test cache
// answers that instantly for an unchanged package, so the GOFLAGS it inherits
// carries -count=1 to make what it multiplies a real measurement.
//
// A coefficient the caller fixed in GREMLINS_FLAGS is the one gremlins
// applies, so it is the one announced, and neither the budget nor the ceiling
// bounds it. Announcing the derived one beside it printed a deadline that was
// not in force, which is exactly the mode issue 1029's timeouts were read in.
func deriveDeadline(pkg, base string, fixed *string, k knobs) (deadline, error) {
	took := number(base)
	if fixed != nil {
		return deadline{notices: []string{fmt.Sprintf("%s coverage run takes %ss by the clock, and GREMLINS_FLAGS fixes -timeout-coefficient %s: about %.1fs per mutant, which neither the %ss budget nor the %ss ceiling bounds",
			pkg, base, *fixed, took*number(*fixed), k.budget, k.ceiling)}}, nil
	}
	coefficient := min(max(wholeMultiple(k.budget, took)+1, coefficientFloor), largestCoefficient)
	limit := min(wholeMultiple(k.ceiling, took), largestCoefficient)
	if limit < 2 {
		return deadline{}, fmt.Errorf("%s's coverage run takes %ss by the clock, and a %ss ceiling does not hold two of them, so every mutant would be reported TIMED OUT; raise MUTANT_DEADLINE_MAX (the fourth argument) to measure it", pkg, base, k.ceiling)
	}
	read := deadline{derived: true}
	if coefficient > limit {
		if limit < coefficientFloor {
			read.notices = append(read.notices, fmt.Sprintf("the %ss ceiling holds the coefficient at %.0f, under the floor of %d a slow package is otherwise given, so a mutant that is only slow under four workers may be reported TIMED OUT", k.ceiling, limit, coefficientFloor))
		} else {
			read.notices = append(read.notices, fmt.Sprintf("the %ss ceiling holds the coefficient at %.0f rather than %.0f", k.ceiling, limit, coefficient))
		}
		coefficient = limit
	}
	read.coefficient = strconv.FormatFloat(coefficient, 'f', 0, 64)
	read.notices = append(read.notices, fmt.Sprintf("%s coverage run takes %ss by the clock, so -timeout-coefficient %s: about %.1fs per mutant (budget %ss, ceiling %ss)",
		pkg, base, read.coefficient, took*coefficient, k.budget, k.ceiling))
	return read, nil
}

// wholeMultiple is how many times took fits in figure. It stays a float, since
// a ceiling of 1e9 s over a base of a third of a second is past what a 32-bit
// integer holds, and every use clamps it first.
func wholeMultiple(figure string, took float64) float64 {
	return math.Floor(number(figure) / took)
}
