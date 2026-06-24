package main

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestNormalizeType_StripsPointerAndCase verifies normalizeType lowercases and
// removes a single leading pointer marker so pointer-vs-value comparisons match.
func TestNormalizeType_StripsPointerAndCase(t *testing.T) {
	cases := map[string]string{
		"*string":          "string",
		"String":           "string",
		" *Int64 ":         "int64",
		"v2.AccessLevel":   "v2.accesslevel",
		"*v2.AccessLevelV": "v2.accesslevelv",
	}
	for input, want := range cases {
		if got := normalizeType(input); got != want {
			t.Errorf("normalizeType(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestScalarLike_ClassifiesGoScalars verifies the scalar allowlist used to treat
// pointer/value scalar projections as compatible.
func TestScalarLike_ClassifiesGoScalars(t *testing.T) {
	for _, ok := range []string{"int", "int64", "int32", "float64", "float32", "bool", "string"} {
		if !scalarLike(ok) {
			t.Errorf("scalarLike(%q) = false, want true", ok)
		}
	}
	for _, no := range []string{"v2.commit", "[]string", "map[string]int", ""} {
		if scalarLike(no) {
			t.Errorf("scalarLike(%q) = true, want false", no)
		}
	}
}

// TestTypesCompatible_AcceptsKnownProjections verifies the compatibility rules:
// pointer-ness ignored, scalar↔scalar accepted, SDK Value enums projected onto
// int/string, and SDK time types projected onto string. Genuinely divergent
// struct types are reported as incompatible.
func TestTypesCompatible_AcceptsKnownProjections(t *testing.T) {
	compatible := [][2]string{
		{"string", "*string"},
		{"int", "int64"},
		{"int", "*v2.AccessLevelValue"},
		{"string", "v2.VisibilityValue"},
		{"string", "*v2.ISOTime"},
		{"string", "time.Time"},
		{"bool", "*bool"},
	}
	for _, pair := range compatible {
		if !typesCompatible(pair[0], pair[1]) {
			t.Errorf("typesCompatible(%q, %q) = false, want true", pair[0], pair[1])
		}
	}
	incompatible := [][2]string{
		{"string", "*v2.Commit"},
		{"int", "[]string"},
		{"v2.Foo", "v2.Bar"},
	}
	for _, pair := range incompatible {
		if typesCompatible(pair[0], pair[1]) {
			t.Errorf("typesCompatible(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

// TestTagValue_PrefersFirstKeyAndStripsOptions verifies tag selection across the
// preferred key order and the stripping of ",omitempty" style suffixes.
func TestTagValue_PrefersFirstKeyAndStripsOptions(t *testing.T) {
	raw := reflect.StructTag(`url:"search,omitempty" json:"search_query,omitempty"`)
	if got := tagValue(raw, []string{"url", "json"}); got != "search" {
		t.Errorf("tagValue url-first = %q, want %q", got, "search")
	}
	if got := tagValue(raw, []string{"json"}); got != "search_query" {
		t.Errorf("tagValue json = %q, want %q", got, "search_query")
	}
	if got := tagValue(reflect.StructTag(`json:"-"`), []string{"json"}); got != "-" {
		t.Errorf("tagValue dash = %q, want %q", got, "-")
	}
	if got := tagValue(reflect.StructTag(``), []string{"json"}); got != "" {
		t.Errorf("tagValue empty = %q, want %q", got, "")
	}
}

// TestPathHelpers verifies the package-name extraction helpers.
func TestPathHelpers(t *testing.T) {
	if got := lastPathSegment("gitlab.com/gitlab-org/api/client-go/v2"); got != "v2" {
		t.Errorf("lastPathSegment = %q, want v2", got)
	}
	if got := lastPathSegment("flat"); got != "flat" {
		t.Errorf("lastPathSegment(flat) = %q, want flat", got)
	}
	full := "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches"
	if got := shortPackage(full); got != "branches" {
		t.Errorf("shortPackage = %q, want branches", got)
	}
	if got := shortPackage("nope"); got != "nope" {
		t.Errorf("shortPackage(nope) = %q, want nope", got)
	}
}

// TestBuildReport_DetectsKnownBranchGaps runs the auditor against the real
// repository and asserts that the known, verified branches gaps are reported.
// This doubles as the methodology regression guard: if the resolver stops
// attributing SDK structs to MCP structs, these assertions fail.
func TestBuildReport_DetectsKnownBranchGaps(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.Summary.Packages == 0 || rep.Summary.InputPairs == 0 || rep.Summary.OutputPairs == 0 {
		t.Fatalf("summary looks empty: %+v", rep.Summary)
	}

	branches := findPackage(t, rep, "branches")
	if branches.InputPairs == 0 || branches.OutputPairs == 0 {
		t.Fatalf("branches has no resolved pairs: %+v", branches)
	}
	// ProtectRepositoryBranchesOptions exposes granular access arrays the MCP
	// ProtectInput does not surface — a real R-INPUT gap.
	assertMissingField(t, branches, "input", "ProtectInput", "allowed_to_push")
	// The Branch result exposes the full commit object; the MCP Output flattens
	// it to commit_id, so `commit` is reported missing — a real R-OUTPUT note.
	assertMissingField(t, branches, "output", "Output", "commit")
}

// TestBuildReport_Deterministic verifies two runs produce identical output, a
// prerequisite for using the report as a committed backlog.
func TestBuildReport_Deterministic(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	first, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("first buildReport: %v", err)
	}
	second, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("second buildReport: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}

func findPackage(t *testing.T, rep report, name string) packageReport {
	t.Helper()
	for _, pr := range rep.Packages {
		if pr.Package == name {
			return pr
		}
	}
	t.Fatalf("package %q not found in report", name)
	return packageReport{}
}

func assertMissingField(t *testing.T, pr packageReport, kind, mcpType, tag string) {
	t.Helper()
	for _, g := range pr.Gaps {
		if g.Kind != kind || g.MCPType != mcpType {
			continue
		}
		for _, mf := range g.MissingFields {
			if mf.Tag == tag {
				return
			}
		}
	}
	t.Errorf("expected %s gap %s missing field %q in package %q, not found", kind, mcpType, tag, pr.Package)
}
