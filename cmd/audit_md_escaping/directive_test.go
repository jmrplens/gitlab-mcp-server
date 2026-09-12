package main

import (
	"strings"
	"testing"
)

// TestParseDirective_Comments_ReadsOnlyAWellFormedExemption covers what counts
// as an exemption and what does not, for both directives. Both halves are
// required: without the reason, the next reader cannot tell a value that
// needs no escaping from one somebody decided not to escape.
func TestParseDirective_Comments_ReadsOnlyAWellFormedExemption(t *testing.T) {
	cases := []struct {
		name           string
		comment        string
		wantKind       directiveKind
		wantExpression string
		wantReason     string
	}{
		{
			name:           "an expression and a reason",
			comment:        "//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in.",
			wantKind:       kindUnescaped,
			wantExpression: "result.ID",
			wantReason:     "a canonical catalog ID, compiled in.",
		},
		{
			name:           "indented in a doc comment",
			comment:        "   //gitlab:allow-unescaped  result.ID :  a reason  ",
			wantKind:       kindUnescaped,
			wantExpression: "result.ID",
			wantReason:     "a reason",
		},
		{
			name:           "the raw directive",
			comment:        "//gitlab:allow-raw item.CreatedAt: a date GitLab sends without a time, shown as the day it names.",
			wantKind:       kindRaw,
			wantExpression: "item.CreatedAt",
			wantReason:     "a date GitLab sends without a time, shown as the day it names.",
		},
		{name: "another comment", comment: "// result.ID is fine"},
		{name: "another directive", comment: "//gitlab:allow-readonly-graphql-mutation x: y"},
		{name: "no reason", comment: "//gitlab:allow-unescaped result.ID"},
		{name: "an empty reason", comment: "//gitlab:allow-unescaped result.ID:   "},
		{name: "no expression", comment: "//gitlab:allow-unescaped : a reason"},
		{name: "a raw directive with no reason", comment: "//gitlab:allow-raw item.CreatedAt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, expression, reason, ok := parseDirective(tc.comment)
			if tc.wantExpression == "" {
				if ok {
					t.Errorf("parseDirective(%q) read an exemption: %s %q / %q", tc.comment, kind, expression, reason)
				}
				return
			}
			if !ok {
				t.Fatalf("parseDirective(%q) read no exemption", tc.comment)
			}
			if kind != tc.wantKind || expression != tc.wantExpression || reason != tc.wantReason {
				t.Errorf("parseDirective(%q) = %s %q / %q, want %s %q / %q",
					tc.comment, kind, expression, reason, tc.wantKind, tc.wantExpression, tc.wantReason)
			}
		})
	}
}

// TestCollectDirectives_Fixture_KeysOnPackageAndExpression checks that an
// exemption is read where it was declared and belongs to the package that
// declared it, so one package cannot excuse another's value.
func TestCollectDirectives_Fixture_KeysOnPackageAndExpression(t *testing.T) {
	prog := loadFixture(t, caseFixture)

	directives := collectDirectives(prog, repoRoot(t))

	key := directiveKey{pkg: fixtureDir + "/mdexempt", kind: kindUnescaped, expression: "result.ID"}
	declared, ok := directives[key]
	if !ok {
		t.Fatalf("the declared exemption was not read; got %v", directives)
	}
	if !strings.Contains(declared.Reason, "canonical catalog ID") {
		t.Errorf("reason %q is not the one the source gives", declared.Reason)
	}
	if !strings.HasSuffix(declared.File, "mdexempt.go") || declared.Line == 0 {
		t.Errorf("exemption at %s:%d does not point at the source that declared it", declared.File, declared.Line)
	}
	if _, wrongPackage := directives[directiveKey{pkg: fixtureDir + "/mdcase", kind: kindUnescaped, expression: "result.ID"}]; wrongPackage {
		t.Error("an exemption was read as belonging to a package that did not declare it")
	}
	if _, wrongKind := directives[directiveKey{pkg: key.pkg, kind: kindRaw, expression: "result.ID"}]; wrongKind {
		t.Error("an escaping exemption was read as answering the raw verdict too")
	}
	// Counted over the fixture alone. collectDirectives reads every package
	// the load pulled in, and the fixture type-checks against the real
	// toolutil, which declares exemptions of its own; counting those here
	// would make this assertion fail whenever a real package gains one.
	var inFixture int
	for key := range directives {
		if strings.HasPrefix(key.pkg, fixtureDir) {
			inFixture++
		}
	}
	if inFixture != 2 {
		t.Errorf("read %d exemptions in the fixture, want the two well-formed ones", inFixture)
	}
}

// TestStaleDirectives_Cases_ReportWhatExcusedNothing checks the half of the
// mechanism that keeps it honest: an exemption nothing used has outlived the
// reason it was written for. A raw exemption is the exception while its rule
// is staged: a run that never asked the raw question cannot say the answer
// was not needed.
func TestStaleDirectives_Cases_ReportWhatExcusedNothing(t *testing.T) {
	declared := map[directiveKey]Directive{
		{pkg: "a", kind: kindUnescaped, expression: "used"}:   {Package: "a", Expression: "used", File: "a.go", Line: 1},
		{pkg: "a", kind: kindUnescaped, expression: "unused"}: {Package: "a", Expression: "unused", File: "a.go", Line: 2},
		{pkg: "a", kind: kindRaw, expression: "raw"}:          {Package: "a", Kind: kindRaw, Expression: "raw", File: "a.go", Line: 3},
	}
	used := map[directiveKey]bool{{pkg: "a", kind: kindUnescaped, expression: "used"}: true}
	cases := []struct {
		name      string
		judgedRaw bool
		want      []string
	}{
		{name: "the raw verdict did not run", judgedRaw: false, want: []string{"unused"}},
		{name: "the raw verdict ran", judgedRaw: true, want: []string{"unused", "raw"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, directive := range staleDirectives(declared, used, tc.judgedRaw) {
				got = append(got, directive.Expression)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("stale = %v, want %v", got, tc.want)
			}
		})
	}
	if len(staleDirectives(declared, map[directiveKey]bool{
		{pkg: "a", kind: kindUnescaped, expression: "used"}:   true,
		{pkg: "a", kind: kindUnescaped, expression: "unused"}: true,
		{pkg: "a", kind: kindRaw, expression: "raw"}:          true,
	}, true)) != 0 {
		t.Error("an exemption that excused something was reported stale")
	}
}

// TestShortPackage_Paths_TrimsTheModule checks that a report names a package
// the way the repository does.
func TestShortPackage_Paths_TrimsTheModule(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "a package of this module", path: modulePath + "/internal/tools/issues", want: "internal/tools/issues"},
		{name: "the module itself", path: modulePath, want: modulePath},
		{name: "another module", path: "example.invalid/other", want: "example.invalid/other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortPackage(tc.path); got != tc.want {
				t.Errorf("shortPackage(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
