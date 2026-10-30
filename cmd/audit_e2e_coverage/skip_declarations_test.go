// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
)

// skipPackages are the packages a complete run tests, which a declaration's
// Package must name.
var skipPackages = map[string]bool{"common": true, "ce": true, "ee": true}

// TestSkipDeclarations_EveryEntry_WellFormed verifies that every skip
// declaration names a run, a package and a test the gate can match, carries a
// fragment of the reason so it cannot excuse the test for another one, and
// says which kind of skip it is and why it stands.
//
// An empty Because would be the dangerous one: every reason contains the empty
// string, so the declaration would excuse the test whatever it skipped for.
func TestSkipDeclarations_EveryEntry_WellFormed(t *testing.T) {
	for index, declaration := range declaredSkips {
		switch {
		case !skipRuntimes[declaration.Runtime]:
			t.Errorf("declaration %d names the run %q, want ce or ee", index, declaration.Runtime)
		case !skipPackages[declaration.Package]:
			t.Errorf("declaration %d names the package %q, want common, ce or ee", index, declaration.Package)
		case !strings.HasPrefix(declaration.Test, "Test") || strings.HasSuffix(declaration.Test, "/"):
			t.Errorf("declaration %d names the test %q, want a Test function or a subtest of one", index, declaration.Test)
		case strings.TrimSpace(declaration.Because) == "":
			t.Errorf("declaration %d of %s carries no fragment of the reason, so it would excuse any skip of that test", index, declaration.Test)
		case !declaredSkipCategories[declaration.Category]:
			t.Errorf("declaration %d of %s has the category %q, which is not a declared one", index, declaration.Test, declaration.Category)
		case strings.TrimSpace(declaration.Reason) == "":
			t.Errorf("declaration %d of %s says nothing about why it stands", index, declaration.Test)
		}
	}
}

// finalPassLine matches one scenario final_pass_test prints, as its case
// branch spells it: the runtime, then the package, the test and the switch.
var finalPassLine = regexp.MustCompile(`(?m)^\s*([a-z]+)\) echo "([a-z]+) (Test\w+) ([A-Z0-9_]+)" ;;$`)

// TestSkipDeclarations_RunsLast_AreWhatTheFinalPassRuns verifies that the
// scenarios a complete run's final pass runs, which final_pass_test in
// test/e2e/scripts/skip-gate.sh prints per runtime, are exactly the runs-last
// declarations of the table, by runtime, package and test. A declaration the
// final pass does not run would fail every complete run of its runtime once
// the main pass was over, and a scenario the final pass runs that nothing
// declares would have its main pass skip fail the run as undeclared.
func TestSkipDeclarations_RunsLast_AreWhatTheFinalPassRuns(t *testing.T) {
	root, err := mcpsurface.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot() error = %v", err)
	}
	script, err := os.ReadFile(filepath.Join(root, "test", "e2e", "scripts", "skip-gate.sh")) // #nosec G304 -- a fixed path under the repository root.
	if err != nil {
		t.Fatalf("read skip-gate.sh: %v", err)
	}
	text := string(script)
	start := strings.Index(text, "\nfinal_pass_test() {\n")
	if start < 0 {
		t.Fatal("skip-gate.sh defines no final_pass_test")
	}
	body, _, closed := strings.Cut(text[start:], "\n}\n")
	if !closed {
		t.Fatal("final_pass_test in skip-gate.sh has no closing brace of its own line")
	}

	var printed []string
	for _, match := range finalPassLine.FindAllStringSubmatch(body, -1) {
		printed = append(printed, match[1]+" "+match[2]+" "+match[3])
	}
	var declared []string
	for _, declaration := range declaredSkips {
		if declaration.Category == skipCategoryRunsLast {
			declared = append(declared, declaration.Runtime+" "+declaration.Package+" "+declaration.Test)
		}
	}
	slices.Sort(printed)
	slices.Sort(declared)
	if !slices.Equal(printed, declared) {
		t.Errorf("final_pass_test runs %v, want the runs-last declarations %v", printed, declared)
	}
	if strings.Count(body, ") echo ") != len(printed) {
		t.Errorf("final_pass_test holds a branch this test cannot read:\n%s", body)
	}
}

// TestSkipDeclarations_NoTwoCoverOneSkip verifies that no two declarations of
// one run name the same test for the same reason, which the gate would report
// only on a run that has that skip: the second declaration covers nothing.
func TestSkipDeclarations_NoTwoCoverOneSkip(t *testing.T) {
	seen := map[skipDeclaration]bool{}
	for _, declaration := range declaredSkips {
		key := skipDeclaration{Runtime: declaration.Runtime, Package: declaration.Package, Test: declaration.Test, Because: declaration.Because}
		if seen[key] {
			t.Errorf("%s %s %s is declared twice for %q", declaration.Runtime, declaration.Package, declaration.Test, declaration.Because)
		}
		seen[key] = true
	}
}
