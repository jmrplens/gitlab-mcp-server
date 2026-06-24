package main

import (
	"reflect"
	"strings"
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

// TestExtraOutputFields_FlagsInventedScalars verifies R-OUTPUT-EXTRA detection:
// an MCP output json tag with no SDK result counterpart is flagged as extra,
// while SDK-backed tags, the MCP-envelope carve-out, and the "-" sentinel are
// not. This is the mechanical guard for the 1:1 "no invented output" rule.
func TestExtraOutputFields_FlagsInventedScalars(t *testing.T) {
	sdkFields := map[string]string{
		"id":        "int",
		"author_id": "int",
		"iids[]":    "[]int", // url-style SDK tag; normalizes to "iids"
		"milestone": "*v2.Milestone",
		"web_url":   "string",
	}
	mcpFields := map[string]string{
		"id":              "int",      // SDK-backed → not extra
		"iids":            "[]int",    // matches normalizeSDKTag("iids[]") → not extra
		"author_username": "string",   // invented scalar → extra
		"milestone_title": "string",   // invented scalar → extra
		"pagination":      "v2.Page",  // envelope carve-out → not extra
		"next_steps":      "[]string", // envelope carve-out → not extra
		"-":               "string",   // sentinel → not extra
	}

	extras := extraOutputFields("SomeOutput", mcpFields, sdkFields)

	gotTags := map[string]bool{}
	for _, e := range extras {
		gotTags[e.Tag] = true
	}
	wantExtra := []string{"author_username", "milestone_title"}
	for _, tag := range wantExtra {
		if !gotTags[tag] {
			t.Errorf("expected %q reported as extra output field, got %v", tag, extras)
		}
	}
	wantNotExtra := []string{"id", "iids", "pagination", "next_steps", "-"}
	for _, tag := range wantNotExtra {
		if gotTags[tag] {
			t.Errorf("did not expect %q reported as extra output field, got %v", tag, extras)
		}
	}
	if len(extras) != len(wantExtra) {
		t.Errorf("extra count = %d (%v), want %d", len(extras), extras, len(wantExtra))
	}
}

// TestExtraOutputFields_OnlyOutputKind verifies diffPair attaches ExtraFields for
// output pairs but never for input pairs (MCP inputs carry path ids legitimately).
func TestExtraOutputFields_DiffPairKindGating(t *testing.T) {
	// Synthesize via the real diffPair against the repository to confirm input
	// gaps never carry extras. Output extras are exercised in the unit test above.
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	for _, pr := range rep.Packages {
		for _, g := range pr.Gaps {
			if g.Kind == "input" && len(g.ExtraFields) > 0 {
				t.Errorf("input gap %s/%s carries extra fields %v, want none", pr.Package, g.MCPType, g.ExtraFields)
			}
		}
	}
}

// TestNonResultStructName_ExcludesWrapperOptionsAndTime verifies the name-based
// non-result classifier that prevents the three R-OUTPUT-EXTRA false-positive
// classes: the pagination Response wrapper, *Options request structs, and the
// ISOTime/Time value types.
func TestNonResultStructName_ExcludesWrapperOptionsAndTime(t *testing.T) {
	nonResult := []string{
		"Response",                       // list pagination wrapper
		"ListIssuesOptions",              // request options struct
		"UpdateSecurityAttributeOptions", // request options struct
		"ISOTime",                        // client-go date value type
		"Time",                           // time.Time value type
	}
	for _, name := range nonResult {
		if !nonResultStructName(name) {
			t.Errorf("nonResultStructName(%q) = false, want true", name)
		}
	}
	result := []string{"Issue", "MergeRequest", "Branch", "PersonalAccessToken", "Response2", "OptionsList"}
	for _, name := range result {
		if nonResultStructName(name) {
			t.Errorf("nonResultStructName(%q) = true, want false", name)
		}
	}
}

// TestIsAcceptedRename_AllowsScopedAndGlobalTags verifies the accepted-rename
// allowlist: a type-scoped entry suppresses only that MCP type's tag, while a
// genuinely invented scalar (not in the allowlist) is still reported.
func TestIsAcceptedRename_AllowsScopedAndGlobalTags(t *testing.T) {
	// Seeded scoped entry: branches Output renames SDK `name` → `branch_name`.
	if !isAcceptedRename("Output", "branch_name") {
		t.Errorf("isAcceptedRename(Output, branch_name) = false, want true (seeded allowlist)")
	}
	// Same tag on a different MCP type is NOT suppressed by the scoped entry.
	if isAcceptedRename("OtherOutput", "branch_name") {
		t.Errorf("isAcceptedRename(OtherOutput, branch_name) = true, want false (scoped to Output)")
	}
	// A genuine invented scalar is never accepted.
	if isAcceptedRename("Output", "invented_field") {
		t.Errorf("isAcceptedRename(Output, invented_field) = true, want false")
	}
}

// TestExtraOutputFields_SuppressesAllowlistedRename verifies extraOutputFields
// honors the accepted-rename allowlist: an allowlisted rename is not reported as
// extra, while a genuinely invented scalar in the same struct still is.
func TestExtraOutputFields_SuppressesAllowlistedRename(t *testing.T) {
	sdkFields := map[string]string{
		"name": "string", // SDK scalar that the MCP renames to branch_name
		"id":   "int",
	}
	mcpFields := map[string]string{
		"branch_name":    "string", // allowlisted rename of SDK `name` → not extra
		"id":             "int",    // SDK-backed → not extra
		"invented_field": "string", // genuine invented scalar → extra
	}

	// Scoped to "Output": branch_name is suppressed.
	extras := extraOutputFields("Output", mcpFields, sdkFields)
	gotTags := map[string]bool{}
	for _, e := range extras {
		gotTags[e.Tag] = true
	}
	if gotTags["branch_name"] {
		t.Errorf("allowlisted rename branch_name reported as extra: %v", extras)
	}
	if !gotTags["invented_field"] {
		t.Errorf("genuine invented scalar invented_field not reported as extra: %v", extras)
	}
	if len(extras) != 1 {
		t.Errorf("extra count = %d (%v), want 1 (only invented_field)", len(extras), extras)
	}

	// On a non-allowlisted MCP type, branch_name IS reported (proves it is the
	// allowlist, not a generic suppression, that hides it above).
	other := extraOutputFields("UnlistedOutput", mcpFields, sdkFields)
	otherTags := map[string]bool{}
	for _, e := range other {
		otherTags[e.Tag] = true
	}
	if !otherTags["branch_name"] {
		t.Errorf("branch_name should be extra on UnlistedOutput (not allowlisted): %v", other)
	}
}

// TestBuildReport_NoFalsePositiveExtras is the real-repo regression guard for the
// three R-OUTPUT-EXTRA false-positive classes. It asserts that:
//
//   - list converters taking the v2.Response pagination wrapper no longer report
//     their element slice (merge_requests, discussions, events, award_emoji,
//     runners) as extra;
//   - converters whose only client-go arg is an *Options struct or a time value
//     type no longer mis-pair (securityattributes/securitycategories show no
//     Options-sourced extras; accesstokens no longer pairs against ISOTime);
//   - the issue family (issues, issuenotes, issuediscussions) reports ZERO
//     extras after the cleanup.
func TestBuildReport_NoFalsePositiveExtras(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}

	// No output pair may be formed against a non-result SDK struct.
	for _, pr := range rep.Packages {
		for _, g := range pr.Gaps {
			if g.Kind != "output" {
				continue
			}
			sdk := g.SDKType
			if sdkLeaf(sdk) == "Response" ||
				strings.HasSuffix(sdk, "Options") ||
				strings.HasSuffix(sdk, "ISOTime") ||
				sdkLeaf(sdk) == "Time" {
				t.Errorf("%s: output pair %s mis-paired against non-result SDK struct %q (extras: %v)",
					pr.Package, g.MCPType, sdk, g.ExtraFields)
			}
		}
	}

	// The issue family must report zero extras after the family cleanup.
	for _, name := range []string{"issues", "issuenotes", "issuediscussions"} {
		pr := findPackage(t, rep, name)
		if pr.ExtraOutputCount != 0 {
			var tags []string
			for _, g := range pr.Gaps {
				for _, e := range g.ExtraFields {
					tags = append(tags, e.Tag)
				}
			}
			t.Errorf("package %q has %d extra output fields, want 0 (tags: %v)", name, pr.ExtraOutputCount, tags)
		}
	}
}

// sdkLeaf returns the type name after the last "." in an SDK type string
// (e.g. "v2.Response" → "Response").
func sdkLeaf(sdk string) string {
	if i := strings.LastIndex(sdk, "."); i >= 0 {
		return sdk[i+1:]
	}
	return sdk
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
