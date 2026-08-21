package structs

import (
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// structField is a synthetic (json-tag, Go scalar type) field used to build the
// minimal *types.Struct values the union/disjoint helpers operate on, without
// loading real packages.
type structField struct {
	name    string // exported Go field name
	jsonTag string
	goType  types.Type
}

// makeStruct builds a *types.Struct with the given json-tagged fields. Field Go
// types are basic scalars so flattenFields records "string"/"int"/etc.
func makeStruct(fields ...structField) *types.Struct {
	vars := make([]*types.Var, 0, len(fields))
	tags := make([]string, 0, len(fields))
	for _, f := range fields {
		vars = append(vars, types.NewField(token.NoPos, nil, f.name, f.goType, false))
		tags = append(tags, `json:"`+f.jsonTag+`"`)
	}
	return types.NewStruct(vars, tags)
}

var (
	tString = types.Typ[types.String]
	tInt    = types.Typ[types.Int]
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

// cachedBuildReport runs the full struct analysis once per gapsOnly mode and
// shares the result across this package's tests: buildReport is a pure
// (~3-5s) analysis over the working tree, so per-test re-runs only multiplied
// CPU time. Tests must treat the returned report as read-only.
// TestBuildReport_Deterministic still performs one fresh run and compares it
// against the cached one, preserving the two-independent-runs property.
var (
	cachedReportOnce [2]sync.Once
	cachedReports    [2]report
	cachedReportErrs [2]error
)

func cachedBuildReport(t *testing.T, gapsOnly bool) report {
	t.Helper()
	idx := 0
	if gapsOnly {
		idx = 1
	}
	cachedReportOnce[idx].Do(func() {
		root, err := cmdutil.RepositoryRoot(".")
		if err != nil {
			cachedReportErrs[idx] = err
			return
		}
		cachedReports[idx], cachedReportErrs[idx] = buildReport(root, gapsOnly)
	})
	if cachedReportErrs[idx] != nil {
		t.Fatalf("buildReport: %v", cachedReportErrs[idx])
	}
	return cachedReports[idx]
}

// TestBuildReport_DetectsKnownBranchGaps runs the auditor against the real
// repository as a methodology regression guard: it verifies the resolver still
// attributes a healthy number of SDK↔MCP input/output pairs across the tools tree
// (a broken resolver would collapse these toward zero).
func TestBuildReport_DetectsKnownBranchGaps(t *testing.T) {
	rep := cachedBuildReport(t, false)
	if rep.Summary.Packages == 0 || rep.Summary.InputPairs == 0 || rep.Summary.OutputPairs == 0 {
		t.Fatalf("summary looks empty: %+v", rep.Summary)
	}

	branches := findPackage(t, rep, "branches")
	if branches.InputPairs == 0 || branches.OutputPairs == 0 {
		t.Fatalf("branches has no resolved pairs: %+v", branches)
	}
	// Methodology regression guard: the resolver must keep attributing many
	// input/output pairs across the tools tree. A broken resolver (e.g. the
	// converter/handler detection stops matching SDK structs to MCP structs) would
	// collapse these counts toward zero. The original "known branches gaps"
	// assertions were removed because branches was migrated to 1:1 in the audit
	// (ProtectInput.allowed_to_push added, Output.commit migrated to an object), so
	// those specific gaps no longer exist.
	if rep.Summary.InputPairs < 50 || rep.Summary.OutputPairs < 50 {
		t.Fatalf("resolver attributed too few pairs (resolver regression?): %+v", rep.Summary)
	}
}

// TestBuildReport_Deterministic verifies two runs produce identical output, a
// prerequisite for using the report as a committed backlog.
func TestBuildReport_Deterministic(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	first := cachedBuildReport(t, true)
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

	extras := extraOutputFields("testpkg", "SomeOutput", mcpFields, sdkFields)

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

// TestExtraOutputFields_DiffPairKindGating verifies diffPair attaches ExtraFields for
// output pairs but never for input pairs (MCP inputs carry path ids legitimately).
func TestExtraOutputFields_DiffPairKindGating(t *testing.T) {
	// Synthesize via the real diffPair against the repository to confirm input
	// gaps never carry extras. Output extras are exercised in the unit test above.
	rep := cachedBuildReport(t, true)
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
	extras := extraOutputFields("testpkg", "Output", mcpFields, sdkFields)
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
	other := extraOutputFields("testpkg", "UnlistedOutput", mcpFields, sdkFields)
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
	rep := cachedBuildReport(t, false)

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

// TestDiffOutputGroup_UnionMultiConverter verifies FIX A: an MCP output type
// produced by two converters paired to a lean and a full SDK struct is diffed
// against the UNION of their fields. A field present only on the full struct is
// neither EXTRA (vs the lean struct) nor MISSING (vs the full struct), and a
// field genuinely absent from BOTH SDK structs is still reported.
func TestDiffOutputGroup_UnionMultiConverter(t *testing.T) {
	// MCP output mirrors the FULL SDK struct plus one invented scalar.
	mcp := makeStruct(
		structField{"ID", "id", tInt},
		structField{"Title", "title", tString},
		structField{"Pipeline", "pipeline", tString}, // only on the full SDK struct
		structField{"Invented", "invented_scalar", tString},
	)
	lean := makeStruct(
		structField{"ID", "id", tInt},
		structField{"Title", "title", tString},
	)
	full := makeStruct(
		structField{"ID", "id", tInt},
		structField{"Title", "title", tString},
		structField{"Pipeline", "pipeline", tString},
		structField{"Author", "author", tString}, // absent from MCP → genuine MISSING
	)

	group := outputGroup{
		mcpName: "Output",
		mcpType: mcp,
		pairs: []structPair{
			{mcpName: "Output", mcpType: mcp, sdkName: "v2.Lean", sdkType: lean},
			{mcpName: "Output", mcpType: mcp, sdkName: "v2.Full", sdkType: full},
		},
	}
	g := diffOutputGroup("testpkg", group)

	if g.SDKType != "v2.Full|v2.Lean" {
		t.Errorf("group SDKType = %q, want joined union %q", g.SDKType, "v2.Full|v2.Lean")
	}

	// "pipeline" is present on the full struct, so it must NOT be extra even though
	// the lean struct lacks it.
	for _, e := range g.ExtraFields {
		if e.Tag == "pipeline" {
			t.Errorf("pipeline reported EXTRA despite being in the union: %v", g.ExtraFields)
		}
	}
	// The only genuine invented scalar must be reported.
	wantExtra := map[string]bool{"invented_scalar": true}
	gotExtra := map[string]bool{}
	for _, e := range g.ExtraFields {
		gotExtra[e.Tag] = true
	}
	if !reflect.DeepEqual(gotExtra, wantExtra) {
		t.Errorf("extra fields = %v, want only invented_scalar", g.ExtraFields)
	}

	// "pipeline" must NOT be missing (present on MCP and in the union); "author"
	// (in the union but absent from MCP) is the only genuine missing field.
	wantMissing := map[string]bool{"author": true}
	gotMissing := map[string]bool{}
	for _, m := range g.MissingFields {
		gotMissing[m.Tag] = true
	}
	if !reflect.DeepEqual(gotMissing, wantMissing) {
		t.Errorf("missing fields = %v, want only author", g.MissingFields)
	}
}

// TestOutputGroups_BucketsByMCPType verifies output pairs sharing one MCP type
// name collapse into a single group whose pairs carry both SDK structs.
func TestOutputGroups_BucketsByMCPType(t *testing.T) {
	a := makeStruct(structField{"ID", "id", tInt})
	b := makeStruct(structField{"ID", "id", tInt})
	mcp := makeStruct(structField{"ID", "id", tInt})
	pairs := map[[2]string]structPair{
		{"Output", "A"}:    {mcpName: "Output", mcpType: mcp, sdkName: "v2.A", sdkType: a},
		{"Output", "B"}:    {mcpName: "Output", mcpType: mcp, sdkName: "v2.B", sdkType: b},
		{"Listed", "List"}: {mcpName: "Listed", mcpType: mcp, sdkName: "v2.List", sdkType: a},
	}
	groups := outputGroups(pairs)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (Listed, Output)", len(groups))
	}
	// Deterministic order: Listed before Output.
	if groups[0].mcpName != "Listed" || groups[1].mcpName != "Output" {
		t.Fatalf("group order = [%s,%s], want [Listed,Output]", groups[0].mcpName, groups[1].mcpName)
	}
	if len(groups[1].pairs) != 2 {
		t.Errorf("Output group has %d pairs, want 2", len(groups[1].pairs))
	}
}

// TestAllIncompatible_RequiresEveryPairing verifies a tag is flagged as a
// mismatch only when the MCP type is incompatible with EVERY SDK type carrying
// that tag; a single compatible pairing suppresses the flag.
func TestAllIncompatible_RequiresEveryPairing(t *testing.T) {
	// Compatible in at least one pairing (string↔string) → not flagged.
	if _, ok := allIncompatible("string", []string{"v2.Commit", "string"}); ok {
		t.Errorf("allIncompatible flagged a tag compatible in one pairing")
	}
	// Incompatible in every pairing → flagged, representative SDK returned.
	sdk, ok := allIncompatible("string", []string{"v2.Commit", "v2.Pipeline"})
	if !ok || sdk != "v2.Commit" {
		t.Errorf("allIncompatible = (%q,%v), want (v2.Commit,true)", sdk, ok)
	}
	// Empty pairing list → not flagged.
	if _, emptyOK := allIncompatible("string", nil); emptyOK {
		t.Errorf("allIncompatible flagged with no SDK types")
	}
}

// TestDisjointPhantomInput_SkipsDisjointHelperOptions verifies FIX B: an input
// struct paired against an Options struct with ZERO field overlap is skipped as
// a phantom ONLY when the same input struct also has a genuinely-overlapping
// Options pairing (the DiffPosition↔PositionOptions case). A disjoint pairing
// that is the input struct's SOLE pairing (path-id-only input vs all-query
// options) is kept as a genuine candidate gap.
func TestDisjointPhantomInput_SkipsDisjointHelperOptions(t *testing.T) {
	// DiffPosition mirrors PositionOptions (full overlap) but a helper also builds
	// ListMergeRequestDiffsOptions (disjoint list options) inside the same func.
	diffPosition := makeStruct(
		structField{"BaseSHA", "base_sha", tString},
		structField{"NewPath", "new_path", tString},
		structField{"NewLine", "new_line", tInt},
	)
	positionOpts := makeStruct(
		structField{"BaseSHA", "base_sha", tString}, // overlaps DiffPosition
		structField{"NewPath", "new_path", tString},
		structField{"NewLine", "new_line", tInt},
	)
	listDiffsOpts := makeStruct(
		structField{"Page", "page", tInt}, // disjoint from DiffPosition
		structField{"PerPage", "per_page", tInt},
		structField{"Sort", "sort", tString},
	)

	genuinePair := structPair{mcpName: "DiffPosition", mcpType: diffPosition, sdkName: "v2.PositionOptions", sdkType: positionOpts, sdkURLTags: true}
	phantomPair := structPair{mcpName: "DiffPosition", mcpType: diffPosition, sdkName: "v2.ListMergeRequestDiffsOptions", sdkType: listDiffsOpts, sdkURLTags: true}

	all := map[[2]string]structPair{
		{"DiffPosition", "PositionOptions"}:              genuinePair,
		{"DiffPosition", "ListMergeRequestDiffsOptions"}: phantomPair,
	}

	if disjointPhantomInput(genuinePair, all) {
		t.Errorf("genuine overlapping pairing (PositionOptions) wrongly flagged as phantom")
	}
	if !disjointPhantomInput(phantomPair, all) {
		t.Errorf("disjoint helper pairing (ListMergeRequestDiffsOptions) not flagged as phantom")
	}

	// A path-id-only input whose ONLY pairing is disjoint must be KEPT (no other
	// overlapping pairing exists), preserving genuine candidate gaps like
	// groups GetInput vs GetGroupOptions.
	getInput := makeStruct(structField{"GroupID", "group_id", tString})
	getGroupOpts := makeStruct(
		structField{"WithProjects", "with_projects", tString},
		structField{"WithCustomAttributes", "with_custom_attributes", tString},
	)
	solePair := structPair{mcpName: "GetInput", mcpType: getInput, sdkName: "v2.GetGroupOptions", sdkType: getGroupOpts, sdkURLTags: true}
	soleAll := map[[2]string]structPair{
		{"GetInput", "GetGroupOptions"}: solePair,
	}
	if disjointPhantomInput(solePair, soleAll) {
		t.Errorf("sole disjoint pairing (GetInput vs GetGroupOptions) wrongly flagged as phantom; genuine candidate gap lost")
	}
}

// TestBuildReport_NoDiffPositionPhantomInput is the real-repo regression guard
// for FIX B: the mrdiscussions/mrdraftnotes DiffPosition input struct must pair
// only against gl.PositionOptions, never the list/diff options a helper builds.
func TestBuildReport_NoDiffPositionPhantomInput(t *testing.T) {
	rep := cachedBuildReport(t, false)
	for _, name := range []string{"mrdiscussions", "mrdraftnotes"} {
		pr := findPackage(t, rep, name)
		for _, g := range pr.Gaps {
			if g.Kind != "input" || g.MCPType != "DiffPosition" {
				continue
			}
			if g.SDKType != "v2.PositionOptions" {
				t.Errorf("%s: DiffPosition paired against %q, want only v2.PositionOptions (phantom not suppressed): missing=%v",
					name, g.SDKType, g.MissingFields)
			}
		}
	}
}

// TestBuildReport_UnionsMultiConverterOutput is the real-repo regression guard
// for FIX A: the mergerequests Output (produced from both BasicMergeRequest and
// MergeRequest) must report a single joined-SDK output gap with no extras and
// none of the MergeRequest-only fields reported as missing.
func TestBuildReport_UnionsMultiConverterOutput(t *testing.T) {
	rep := cachedBuildReport(t, false)
	mr := findPackage(t, rep, "mergerequests")
	var outputs []gap
	for _, g := range mr.Gaps {
		if g.Kind == "output" && g.MCPType == "Output" {
			outputs = append(outputs, g)
		}
	}
	if len(outputs) != 1 {
		t.Fatalf("mergerequests Output produced %d output gaps, want 1 unioned gap: %+v", len(outputs), outputs)
	}
	g := outputs[0]
	if !strings.Contains(g.SDKType, "|") {
		t.Errorf("unioned Output SDKType = %q, want joined names (A|B)", g.SDKType)
	}
	if len(g.ExtraFields) != 0 {
		t.Errorf("mergerequests Output has %d extra fields after union, want 0: %v", len(g.ExtraFields), g.ExtraFields)
	}
	// MergeRequest-only fields must not surface as missing on the union.
	for _, m := range g.MissingFields {
		switch m.Tag {
		case "head_pipeline", "pipeline", "user", "work_in_progress":
			t.Errorf("MergeRequest-only field %q wrongly reported missing on union", m.Tag)
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

// TestDocGroundedSuppression verifies the doc-grounded carve-outs: a curated
// reference subset suppresses missing-field reporting for the whole nested type,
// and a doc-omitted field suppresses a single top-level SDK field — both keyed by
// "<package>.<type>" / "<package>.<type>.<tag>" so they never leak across packages.
func TestDocGroundedSuppression(t *testing.T) {
	if !isCuratedRefSubset("environments", "DeployableOutput") {
		t.Error("DeployableOutput should be a curated ref subset in environments")
	}
	if isCuratedRefSubset("otherpkg", "DeployableOutput") {
		t.Error("curated ref subset must be package-scoped, not global")
	}
	if !isDocOmittedField("environments", "Output", "project") {
		t.Error("environments.Output.project should be a doc-omitted field")
	}
	if isDocOmittedField("environments", "Output", "name") {
		t.Error("name is documented and must not be treated as doc-omitted")
	}
}

// TestBuildReport_AuditedPairFloors pins the number of SDK↔MCP struct pairs
// the R-OUTPUT/R-INPUT resolver attributes, globally and for every package
// whose output shapes are shared through internal/toolutil. The pairing is
// converter-driven: each domain must keep a thin package-local converter
// (possibly a one-line delegate to the toolutil constructor, returning the
// local alias type) or its shapes silently fall out of 1:1 audit coverage —
// exactly the regression this guard exists to catch (output pairs once
// dropped 323→300 when the dedup pass deleted local converters in favor of
// direct toolutil.New* calls, and no audit or test noticed).
//
// If a floor fails after adding or intentionally removing tools, first check
// that every shared-shape package still declares its local converter
// wrappers; only lower a floor when the pair genuinely no longer exists (a
// removed action/shape), and raise it when new pairs are added so the guard
// stays tight.
func TestBuildReport_AuditedPairFloors(t *testing.T) {
	rep := cachedBuildReport(t, false)

	// Floor history: 554 → 545 with Go 1.27's promoted-field composite
	// literals (modernize/embedlit rewrote `Opts{ListOptions: gl.ListOptions{…}}`
	// to promoted fields, removing the inner literal). The dropped pairs were
	// the redundant `input × gl.ListOptions` keys those inner literals minted;
	// each survives as the outer `input × gl.List*Options` pair, whose
	// flattenFields already audits the embedded ListOptions fields.
	const minInputPairs, minOutputPairs = 545, 324
	if rep.Summary.InputPairs < minInputPairs {
		t.Errorf("summary input_pairs = %d, want >= %d (resolver or handler-detection regression)", rep.Summary.InputPairs, minInputPairs)
	}
	if rep.Summary.OutputPairs < minOutputPairs {
		t.Errorf("summary output_pairs = %d, want >= %d (converter-detection regression: a shared toolutil shape may have lost its package-local converter wrapper)", rep.Summary.OutputPairs, minOutputPairs)
	}

	// Output-pair floors for the packages whose shapes are toolutil-shared:
	// these are the ones that go blind first when a local converter is removed.
	outputFloors := map[string]int{
		"boards":              8,
		"branches":            4,
		"commits":             6,
		"enterpriseusers":     2,
		"groupboards":         7,
		"groupmembers":        6,
		"groups":              11,
		"impersonationtokens": 2,
		"issuelinks":          14,
		"members":             3,
		"mergerequests":       5,
		"resourceevents":      9,
		"users":               5,
	}
	for name, floor := range outputFloors {
		pr := findPackage(t, rep, name)
		if pr.OutputPairs < floor {
			t.Errorf("%s output_pairs = %d, want >= %d (lost audited pair; check the package-local converter wrappers for its toolutil-shared shapes)", name, pr.OutputPairs, floor)
		}
	}
}
