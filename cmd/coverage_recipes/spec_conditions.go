// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// specsFile is the file of a domain the gate reads the conditions of, and
// specsPattern the ones a branch may change.
const (
	specsFile    = "action_specs.go"
	specsPattern = "internal/tools/*/action_specs.go"
)

// declaration answers a condition that genuinely cannot take the other value,
// on its own line or on the line above it, and is held to the discipline every
// declaration table here is held to: one that answers nothing fails.
//
//	if meta.usage != "" { // gobco: always true because every entry sets it
const declaration = "// gobco:"

// defaultBase is what a branch's changes are read against when no base is
// given.
const defaultBase = "origin/main"

// notMeasuredReason is a line that says why a package was not measured: the
// first one gobco panicked with, or the recipe's own verdict on the run,
// rather than the first line of output, which a staged run opens with its
// notice. Every verdict the recipe gives a run it will not pass on says either
// that it refuses or that the figure is not a measurement: a package it will
// not load, a report of no condition, and a gobco that exited non-zero, staged
// or not.
var notMeasuredReason = regexp.MustCompile(`(?m)^(panic: |gobco: .*(refusing to|is not a measurement)).*$`)

// figureLine is the line gobco ends its report on.
var figureLine = regexp.MustCompile(`(?m)^Condition coverage: .*$`)

// specGate is one run of the gate over action specs: its session, the git it
// asks, the repository's root, and how many findings it has printed.
type specGate struct {
	*session

	git      string
	root     string
	findings int
}

// specConditions is the spec-conditions subcommand: condition coverage over
// the action_specs.go of the packages a branch touches, which is the one shape
// mutation testing keeps finding and no gate reads: a per-action metadata
// table plus a dispatch on the tool name, where every entry of the table fills
// every field, so each `if meta.x != ""` arm is always taken and its fallback
// can never publish anything. The condition is a decoration, and the mapping
// from action to metadata is stated nowhere a reader can check.
//
// gobco is what sees it: it instruments every boolean condition and reports
// the ones no test ever evaluated both ways. It exits 0 whatever it finds, so
// a gate has to read its output, which is what this does, and so it runs the
// recipe with no gate of the recipe's own, in the mode whose figure it reads,
// whatever the environment sets.
//
// Three deliberate bounds, each because the alternative is worse:
//
//   - Changed packages only. gobco instruments into a temporary tree and
//     caches nothing, so it costs 60 to 80 seconds per package: the 149
//     packages whose action_specs.go this reads would be about three hours.
//
//   - action_specs.go only. A condition elsewhere in a package is the mutation
//     sweep's subject and is reported by `make coverage-conditions`; the claim
//     here is narrow on purpose, so that it can gate.
//
//   - A package gobco cannot instrument is reported and does not fail, since
//     failing on it would fail for the tool's limits rather than for anything
//     about this repository. gobco resolves the external test package against
//     the non-test files alone, so a package whose export_test.go hands a
//     symbol to its _test package died with "undefined": twenty-two of those
//     149 were in that state when the whole tree was measured on 26 October
//     2026 (docs/development/testing/README.md, The whole tree). The recipe
//     stages such a package now, and measures it.
func (s *session) specConditions(ctx context.Context, args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(s.stderr, "%s: spec-conditions takes one base at most\n", toolName)
		return 2
	}
	base := cmp.Or(strings.Join(args, ""), defaultBase)
	g := &specGate{session: s}
	status := g.check(ctx, base)
	if interrupted, hit := recipe.Status(ctx); hit {
		return interrupted
	}
	return status
}

// sayf prints one message of the gate to stdout.
func (g *specGate) sayf(format string, args ...any) {
	fmt.Fprintf(g.stdout, "check-spec-conditions: "+format+"\n", args...)
}

// failf prints one message of the gate to stderr.
func (g *specGate) failf(format string, args ...any) {
	fmt.Fprintf(g.stderr, "check-spec-conditions: "+format+"\n", args...)
}

// report prints one finding and counts it.
func (g *specGate) report(finding string) {
	fmt.Fprintf(g.stderr, "  %s\n", finding)
	g.findings++
}

// gitOutput runs git in dir and returns what it wrote to its standard output.
func (g *specGate) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := recipe.Output(ctx, g.Run, recipe.Call{Program: g.git, Args: args, Dir: dir, Env: g.environ, Stderr: g.stderr})
	return recipe.Line(out), err
}

// check measures what changed against base and returns the gate's status: 2
// when the changes could not be read, 1 when a finding was printed.
func (g *specGate) check(ctx context.Context, base string) int {
	changed, err := g.changed(ctx, base)
	if err != nil {
		g.failf("%v", err)
		return 2
	}
	if len(changed) == 0 {
		g.sayf("no action_specs.go changed against %s", base)
		return 0
	}
	for _, file := range changed {
		if ctx.Err() != nil {
			return 1
		}
		if err = g.checkFile(ctx, file); err != nil {
			g.failf("%v", err)
			return 2
		}
	}
	if g.findings > 0 {
		g.failf("%d finding(s)", g.findings)
		return 1
	}
	g.sayf("every measured condition is evaluated both ways")
	return 0
}

// changed finds git, the repository's root and the files the branch changed
// against base, sorted.
func (g *specGate) changed(ctx context.Context, base string) ([]string, error) {
	git, err := g.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("find git: %w", err)
	}
	g.git = git
	if g.root, err = g.gitOutput(ctx, g.dir, "rev-parse", "--show-toplevel"); err != nil {
		return nil, fmt.Errorf("the repository's root could not be found: %w", err)
	}
	if _, err = g.gitOutput(ctx, g.root, "rev-parse", "--verify", "--quiet", base); err != nil {
		return nil, fmt.Errorf("no such ref: %s", base)
	}
	listed, err := g.gitOutput(ctx, g.root, "diff", "--name-only", base+"...HEAD", "--", specsPattern)
	if err != nil {
		return nil, fmt.Errorf("the changes against %s could not be read: %w", base, err)
	}
	files := strings.Fields(listed)
	slices.Sort(files)
	return slices.Compact(files), nil
}

// checkFile measures the package of one changed file and reports every
// condition of it no test decides and no declaration answers, and every
// declaration that answers none.
func (g *specGate) checkFile(ctx context.Context, file string) error {
	text, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(file)))
	if errors.Is(err, os.ErrNotExist) {
		g.sayf("%s was removed, nothing to measure", file)
		return nil
	}
	if err != nil {
		return err
	}
	pkg := path.Dir(file)
	g.sayf("measuring %s", pkg)
	output, measured := g.measurePackage(ctx, pkg)
	if !measured {
		if ctx.Err() == nil {
			fmt.Fprintf(g.stdout, "  not measured: gobco could not instrument this package: %s\n", reasonOf(output))
		}
		return nil
	}
	for _, figure := range figureLine.FindAllString(output, -1) {
		fmt.Fprintln(g.stdout, figure)
	}
	lines := strings.Split(string(text), "\n")
	answered := 0
	for _, condition := range conditions.OneWay(output) {
		position, named := strings.CutPrefix(condition, specsFile+":")
		if !named {
			continue
		}
		if declared(lines, position) {
			answered++
			continue
		}
		g.report(pkg + "/" + condition)
	}
	declarations := 0
	for _, line := range lines {
		if strings.Contains(line, declaration) {
			declarations++
		}
	}
	// A declaration that answers nothing has stopped describing the package.
	if declarations > answered {
		g.report(fmt.Sprintf("%s: %d declaration(s) and %d answered condition(s); the rest are stale", pkg, declarations, answered))
	}
	return nil
}

// measurePackage runs the condition recipe over a package and returns
// everything it printed, and whether it measured.
func (g *specGate) measurePackage(ctx context.Context, pkg string) (output string, measured bool) {
	var printed bytes.Buffer
	status := g.measure(ctx, g.Tools, conditions.Request{
		Dir: g.root, Package: "./" + pkg,
		Environ: append(slices.Clone(g.environ), "GOBCO_GATE=", "GOBCO_MODE=condition"),
		Stdout:  &printed, Stderr: &printed,
	})
	return strings.ReplaceAll(printed.String(), "\r", ""), status == conditions.Measured
}

// reasonOf is why a package was not measured, out of what its run printed, and
// its first line where nothing in it says.
func reasonOf(output string) string {
	if reason := notMeasuredReason.FindString(output); reason != "" {
		return reason
	}
	first, _, _ := strings.Cut(output, "\n")
	return first
}

// declared reports whether the source line a condition's position names, or
// the line above it, carries a declaration.
func declared(lines []string, position string) bool {
	number, _, _ := strings.Cut(position, ":")
	// Zero for what is no line, above which there is none.
	at, _ := strconv.Atoi(number)
	if at > len(lines) {
		return false
	}
	return slices.ContainsFunc(lines[max(at-2, 0):at], func(line string) bool { return strings.Contains(line, declaration) })
}
