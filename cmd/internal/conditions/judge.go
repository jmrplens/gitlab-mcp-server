// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// oneWay is a condition gobco could not decide both ways, in the three shapes
// its report names one in, the file first and relative to the package it ran
// in:
//
//	main.go:91:5: condition "x != \"\"" was 3 times true but never false
//	main.go:91:5: condition "x != \"\"" was once false but never true
//	main.go:91:5: condition "x != \"\"" was never evaluated
//
// The third is a gap as much as the other two: a condition no test reaches is
// one no test has decided either way.
var oneWay = regexp.MustCompile(`^[^:]+\.go:[0-9]+:[0-9]+: condition .*(but never (true|false)|was never evaluated)$`)

// OneWay returns the lines of a report that name a condition not evaluated
// both ways. A carriage return a Windows console may have added to a line is
// dropped first.
func OneWay(report string) []string {
	var lines []string
	for line := range strings.SplitSeq(strings.ReplaceAll(report, "\r", ""), "\n") {
		if oneWay.MatchString(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// judge refuses a report that measured nothing, and holds one that measured
// to the gate, if there is one.
func (m *measurement) judge(ctx context.Context, report string) int {
	figure := regexp.MustCompile(`(?m)^` + m.settings.figure + `: [0-9]+/[1-9][0-9]*$`)
	if !figure.MatchString(strings.ReplaceAll(report, "\r", "")) {
		m.refusef(ctx, "the report above measured no condition of %s (%s: 0/0, or no such figure at all), which is what gobco prints when no test wrote its counts or when it declined to instrument every file; refusing to pass it on as a measurement", m.Package, m.settings.figure)
		return NotMeasured
	}
	if m.settings.gate == "" {
		return Measured
	}
	var held []string
	elsewhere := 0
	for _, line := range OneWay(report) {
		if m.settings.gate == gateAll || slices.Contains(m.gated, fileOf(line)) {
			held = append(held, line)
		} else {
			elsewhere++
		}
	}
	return m.verdict(ctx, held, elsewhere)
}

// fileOf is the name of the file a condition line opens with, its directory
// dropped under either separator.
func fileOf(line string) string {
	file, _, _ := strings.Cut(line, ":")
	return file[strings.LastIndexAny(file, `/\`)+1:]
}

// verdict says what the gate held, and how many conditions it left to a run
// on the platform it compares this build with.
func (m *measurement) verdict(ctx context.Context, held []string, elsewhere int) int {
	scope := ""
	if m.settings.gate == gateAll {
		scope = fmt.Sprintf("the files of %s measured on %s", m.Package, m.context())
	} else if len(m.gated) > 0 {
		scope = fmt.Sprintf("the files of %s that %s builds and %s does not (%s)", m.Package, m.context(), m.settings.reference, joined(m.gated))
	}
	if len(held) > 0 {
		m.refusef(ctx, "GOBCO_GATE=%s: %d condition(s) in %s were not evaluated both ways:\n  %s", m.settings.gate, len(held), scope, strings.Join(held, "\n  "))
		return GateHeld
	}
	left := ""
	if elsewhere > 0 {
		left = fmt.Sprintf("; %d condition(s) in files %s also builds were not, and are left to a run there", elsewhere, m.settings.reference)
	}
	if scope == "" {
		m.sayf("GOBCO_GATE=%s: %s builds no source of %s that %s does not, so the gate holds nothing here%s", m.settings.gate, m.context(), m.Package, m.settings.reference, left)
	} else {
		m.sayf("GOBCO_GATE=%s: every condition in %s was evaluated both ways%s", m.settings.gate, scope, left)
	}
	return Measured
}
