package structs

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestRun_MarshalFailure_IsReported reaches the encoding branch through the
// seam, since a report of strings and ints never fails to encode on its own.
func TestRun_MarshalFailure_IsReported(t *testing.T) {
	original := marshalIndent
	t.Cleanup(func() { marshalIndent = original })
	marshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("boom") }

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	if _, err = Run(root, true); err == nil || !strings.Contains(err.Error(), "marshal report: boom") {
		t.Fatalf("Run = %v, want the marshal failure", err)
	}
}

// TestHandlerShapes_WithoutBodyOrParams_AreSkipped verifies the two guards on
// a handler declaration: one with no body records no input pair, and one
// whose parameter list is absent (a shape the parser never produces, since it
// always attaches an empty list) has no input struct.
func TestHandlerShapes_WithoutBodyOrParams_AreSkipped(t *testing.T) {
	pkg := &packages.Package{PkgPath: "example.com/x/internal/tools/p"}
	bodyless := &ast.FuncDecl{Name: ast.NewIdent("Handle"), Type: &ast.FuncType{Params: &ast.FieldList{}}}
	pairs := map[[2]string]structPair{}
	collectHandlerInputs(pkg, bodyless, pairs)
	if len(pairs) != 0 {
		t.Errorf("a bodyless handler recorded %d pairs, want none", len(pairs))
	}

	noParams := &ast.FuncDecl{Name: ast.NewIdent("Handle"), Type: &ast.FuncType{}}
	if _, _, ok := handlerInputStruct(pkg, noParams); ok {
		t.Error("a declaration without a parameter list reported an input struct")
	}
}

// TestLocalOrAliasNamedStruct_AliasOfNonStruct_IsNotAPair verifies a
// converter whose result is a local alias of a non-struct type (a slice,
// say) is not paired: the alias is followed, finds no struct, and the
// converter is dropped rather than mis-paired.
func TestLocalOrAliasNamedStruct_AliasOfNonStruct_IsNotAPair(t *testing.T) {
	const pkgPath = "example.com/x/internal/tools/p"
	localPkg := types.NewPackage(pkgPath, "p")
	aliasObj := types.NewTypeName(0, localPkg, "Output", nil)
	alias := types.NewAlias(aliasObj, types.NewSlice(types.Typ[types.String]))
	ident := ast.NewIdent("Output")
	pkg := &packages.Package{
		PkgPath:   pkgPath,
		TypesInfo: &types.Info{Uses: map[*ast.Ident]types.Object{ident: aliasObj}},
	}
	if named, _, name, ok := localOrAliasNamedStruct(pkg, ident, alias); ok || name != "" || named != nil {
		t.Errorf("localOrAliasNamedStruct = %v/%q/%v, want no pair for an alias of a slice", named, name, ok)
	}
}

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
		t.Run(input, func(t *testing.T) {
			if got := normalizeType(input); got != want {
				t.Errorf("normalizeType(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestScalarLike_ClassifiesGoScalars verifies the scalar allowlist used to treat
// pointer/value scalar projections as compatible.
func TestScalarLike_ClassifiesGoScalars(t *testing.T) {
	for _, ok := range []string{"int", "int64", "int32", "float64", "float32", "bool", "string"} {
		t.Run(ok, func(t *testing.T) {
			if !scalarLike(ok) {
				t.Errorf("scalarLike(%q) = false, want true", ok)
			}
		})
	}
	for _, no := range []string{"v2.commit", "[]string", "map[string]int", ""} {
		t.Run(no, func(t *testing.T) {
			if scalarLike(no) {
				t.Errorf("scalarLike(%q) = true, want false", no)
			}
		})
	}
}

// TestTypesCompatible_AcceptsKnownProjections verifies the compatibility rules:
// pointer-ness ignored, scalar↔scalar accepted, SDK Value enums projected onto
// int/string, and SDK time types projected onto string. Genuinely divergent
// struct types are reported as incompatible.
func TestTypesCompatible_AcceptsKnownProjections(t *testing.T) {
	compatible := []struct {
		name, mcpType, sdkType string
	}{
		{"string_to_pointer", "string", "*string"},
		{"int_to_int64", "int", "int64"},
		{"int_to_access_level_pointer", "int", "*v2.AccessLevelValue"},
		{"string_to_visibility", "string", "v2.VisibilityValue"},
		{"string_to_isotime_pointer", "string", "*v2.ISOTime"},
		{"string_to_time", "string", "time.Time"},
		{"bool_to_pointer", "bool", "*bool"},
	}
	for _, tc := range compatible {
		t.Run(tc.name, func(t *testing.T) {
			if !typesCompatible(tc.mcpType, tc.sdkType) {
				t.Errorf("typesCompatible(%q, %q) = false, want true", tc.mcpType, tc.sdkType)
			}
		})
	}
	incompatible := []struct {
		name, mcpType, sdkType string
	}{
		{"string_vs_struct_pointer", "string", "*v2.Commit"},
		{"int_vs_string_slice", "int", "[]string"},
		{"distinct_structs", "v2.Foo", "v2.Bar"},
	}
	for _, tc := range incompatible {
		t.Run(tc.name, func(t *testing.T) {
			if typesCompatible(tc.mcpType, tc.sdkType) {
				t.Errorf("typesCompatible(%q, %q) = true, want false", tc.mcpType, tc.sdkType)
			}
		})
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

// TestCollectPairs_Repository_ListsThePairsTheDiffRunsOver verifies the
// exported pair listing the enum rule builds on: for every package it yields
// exactly the input pairs analyzePackage keeps (phantoms dropped) followed by
// its output pairs, each carrying its kind, both struct sides and the SDK
// tag preference, in a deterministic order.
func TestCollectPairs_Repository_ListsThePairsTheDiffRunsOver(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	// Determinism is judged on the pairs' identities: a Pair carries go/types
	// values, which are not what two runs have to agree on and which the
	// deepequalerrors vet check refuses to compare.
	identities := func(pairs []Pair) []string {
		out := make([]string, 0, len(pairs))
		for _, pair := range pairs {
			out = append(out, pair.Kind+" "+pair.MCPName+" "+pair.SDKName)
		}
		return out
	}
	checked := 0
	for _, pkg := range pkgs {
		pr, ok := analyzePackage(pkg)
		if !ok {
			continue
		}
		checked++
		pairs := CollectPairs(pkg)
		inputs, outputs := countPairKinds(t, pr.Package, pairs)
		if inputs != pr.InputPairs || outputs != pr.OutputPairs {
			t.Errorf("%s: CollectPairs = %d inputs / %d outputs, analyzePackage = %d / %d", pr.Package, inputs, outputs, pr.InputPairs, pr.OutputPairs)
		}
		if again := CollectPairs(pkg); !reflect.DeepEqual(identities(pairs), identities(again)) {
			t.Errorf("%s: CollectPairs is not deterministic", pr.Package)
		}
	}
	if checked < 50 {
		t.Fatalf("checked %d packages, want the resolver to attribute pairs across the tree", checked)
	}
}

// countPairKinds checks each pair's shape and returns how many inputs and
// outputs a package's listing carries, reporting an output that precedes an
// input, a pair of the wrong tag preference, or an incomplete pair.
func countPairKinds(t *testing.T, pkgName string, pairs []Pair) (inputs, outputs int) {
	t.Helper()
	for i, pair := range pairs {
		switch {
		case pair.Kind == "input" && outputs > 0:
			t.Errorf("%s: input pair %s after an output pair", pkgName, pair.MCPName)
		case pair.Kind == "input" && !pair.SDKURLTags:
			t.Errorf("%s: input pair %s does not prefer url tags", pkgName, pair.MCPName)
		case pair.Kind == "output" && pair.SDKURLTags:
			t.Errorf("%s: output pair %s prefers url tags", pkgName, pair.MCPName)
		case pair.Kind != "input" && pair.Kind != "output":
			t.Errorf("%s: pair %d has kind %q", pkgName, i, pair.Kind)
		}
		if pair.Kind == "input" {
			inputs++
		} else {
			outputs++
		}
		if pair.MCPType == nil || pair.SDKType == nil || pair.MCPName == "" || pair.SDKName == "" {
			t.Errorf("%s: pair %d is incomplete: %+v", pkgName, i, pair)
		}
	}
	return inputs, outputs
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
		t.Run(tag, func(t *testing.T) {
			if !gotTags[tag] {
				t.Errorf("expected %q reported as extra output field, got %v", tag, extras)
			}
		})
	}
	wantNotExtra := []string{"id", "iids", "pagination", "next_steps", "-"}
	for _, tag := range wantNotExtra {
		t.Run(tag, func(t *testing.T) {
			if gotTags[tag] {
				t.Errorf("did not expect %q reported as extra output field, got %v", tag, extras)
			}
		})
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
		t.Run(name, func(t *testing.T) {
			if !nonResultStructName(name) {
				t.Errorf("nonResultStructName(%q) = false, want true", name)
			}
		})
	}
	result := []string{"Issue", "MergeRequest", "Branch", "PersonalAccessToken", "Response2", "OptionsList"}
	for _, name := range result {
		t.Run(name, func(t *testing.T) {
			if nonResultStructName(name) {
				t.Errorf("nonResultStructName(%q) = true, want false", name)
			}
		})
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
		t.Run(name, func(t *testing.T) {
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
		})
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
		t.Run(name, func(t *testing.T) {
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
		})
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
		t.Run(name, func(t *testing.T) {
			pr := findPackage(t, rep, name)
			if pr.OutputPairs < floor {
				t.Errorf("%s output_pairs = %d, want >= %d (lost audited pair; check the package-local converter wrappers for its toolutil-shared shapes)", name, pr.OutputPairs, floor)
			}
		})
	}
}

// taggedField is a synthetic struct field carrying a raw struct tag, for the
// helpers that read url tags rather than json ones.
type taggedField struct {
	name   string
	tag    string
	goType types.Type
}

// makeStructWithTags builds a *types.Struct whose fields carry raw tags.
func makeStructWithTags(fields ...taggedField) *types.Struct {
	vars := make([]*types.Var, 0, len(fields))
	tags := make([]string, 0, len(fields))
	for _, f := range fields {
		vars = append(vars, types.NewField(token.NoPos, nil, f.name, f.goType, false))
		tags = append(tags, f.tag)
	}
	return types.NewStruct(vars, tags)
}

// namedStruct declares a named struct type in pkg, for the type-shape helpers
// that read go/types values directly instead of loading a package.
func namedStruct(pkg *types.Package, name string, st *types.Struct) *types.Named {
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), st, nil)
}

// TestDerefNamedStruct_Types_UnwrapsOnePointer verifies the resolver accepts a
// named struct directly or behind one pointer and rejects everything else: a
// nil type, a basic type, and a named type whose underlying is not a struct.
func TestDerefNamedStruct_Types_UnwrapsOnePointer(t *testing.T) {
	pkg := types.NewPackage("example.com/sdk", "sdk")
	branch := namedStruct(pkg, "Branch", makeStruct(structField{name: "Name", jsonTag: "name", goType: tString}))
	alias := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "ISOTime", nil), tString, nil)

	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "nil_type", typ: nil, want: false},
		{name: "basic_type", typ: tString, want: false},
		{name: "named_non_struct", typ: alias, want: false},
		{name: "pointer_to_named_non_struct", typ: types.NewPointer(alias), want: false},
		{name: "named_struct", typ: branch, want: true},
		{name: "pointer_to_named_struct", typ: types.NewPointer(branch), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named, st, ok := derefNamedStruct(tc.typ)
			if ok != tc.want {
				t.Fatalf("derefNamedStruct = %v, want %v", ok, tc.want)
			}
			if ok && (named != branch || st == nil) {
				t.Errorf("derefNamedStruct returned %v/%v, want the Branch struct", named, st)
			}
		})
	}
}

// TestStructUnder_Types_ReachesTheStructThroughPointerAndName verifies the
// embedded-field walk reaches a struct given inline, named, or behind a
// pointer, and reports nothing for a non-struct.
func TestStructUnder_Types_ReachesTheStructThroughPointerAndName(t *testing.T) {
	pkg := types.NewPackage("example.com/sdk", "sdk")
	inline := makeStruct(structField{name: "ID", jsonTag: "id", goType: tInt})
	named := namedStruct(pkg, "ListOptions", inline)

	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "inline_struct", typ: inline, want: true},
		{name: "named_struct", typ: named, want: true},
		{name: "pointer_to_named_struct", typ: types.NewPointer(named), want: true},
		{name: "pointer_to_inline_struct", typ: types.NewPointer(inline), want: true},
		{name: "basic_type", typ: tString, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, ok := structUnder(tc.typ)
			if ok != tc.want {
				t.Fatalf("structUnder = %v, want %v", ok, tc.want)
			}
			if ok && st != inline {
				t.Error("structUnder returned a different struct than the one declared")
			}
		})
	}
}

// TestSDKTypeName_Packages_QualifyWithTheLastSegment verifies the SDK type
// name reported in a gap is qualified by the package's last path segment, and
// that a type with no package (a synthetic or universe type) keeps its bare
// name instead of gaining an empty qualifier.
func TestSDKTypeName_Packages_QualifyWithTheLastSegment(t *testing.T) {
	cases := []struct {
		name string
		pkg  *types.Package
		want string
	}{
		{name: "versioned_sdk_path", pkg: types.NewPackage("example.com/api/client-go/v2", "sdk"), want: "v2.Branch"},
		{name: "single_segment_path", pkg: types.NewPackage("sdk", "sdk"), want: "sdk.Branch"},
		{name: "no_package", pkg: nil, want: "Branch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named := namedStruct(tc.pkg, "Branch", makeStruct())
			if got := sdkTypeName(named); got != tc.want {
				t.Errorf("sdkTypeName = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNonResultSDKStruct_Types_ExcludeWrappersAndTimeValues verifies the
// name-based rule and the package-based one together: an SDK result struct is
// audited, while the pagination wrapper, an *Options request struct and any
// struct from the standard time package are not.
func TestNonResultSDKStruct_Types_ExcludeWrappersAndTimeValues(t *testing.T) {
	sdk := types.NewPackage("example.com/api/client-go/v2", "sdk")
	timePkg := types.NewPackage("time", "time")

	cases := []struct {
		name string
		typ  *types.Named
		want bool
	}{
		{name: "result_struct", typ: namedStruct(sdk, "Branch", makeStruct()), want: false},
		{name: "pagination_wrapper", typ: namedStruct(sdk, "Response", makeStruct()), want: true},
		{name: "options_struct", typ: namedStruct(sdk, "ListBranchesOptions", makeStruct()), want: true},
		{name: "time_package_struct", typ: namedStruct(timePkg, "Duration", makeStruct()), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nonResultSDKStruct(tc.typ); got != tc.want {
				t.Errorf("nonResultSDKStruct(%s) = %v, want %v", tc.typ.Obj().Name(), got, tc.want)
			}
		})
	}
}

// TestFlattenInto_Nesting_StopsAtNilAndDepth verifies the field flattener's
// two guards: a nil struct contributes nothing, and embedding deeper than the
// recursion limit stops rather than descending forever.
func TestFlattenInto_Nesting_StopsAtNilAndDepth(t *testing.T) {
	out := map[string]string{}
	flattenInto(nil, []string{tagKeyJSON}, out, 0)
	if len(out) != 0 {
		t.Errorf("flattenInto(nil) wrote %v, want nothing", out)
	}

	// Eight nested embedded structs: the innermost tag sits below the depth
	// limit of 6 and must not appear.
	deepest := makeStruct(structField{name: "Deep", jsonTag: "deep", goType: tString})
	nested := deepest
	for range 8 {
		embedded := types.NewField(token.NoPos, nil, "Embedded", nested, true)
		nested = types.NewStruct([]*types.Var{embedded}, []string{""})
	}
	if got := flattenFields(nested, []string{tagKeyJSON}); len(got) != 0 {
		t.Errorf("flattenFields descended past the depth limit: %v", got)
	}

	// The same field one level in is reached.
	shallow := types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "Embedded", deepest, true)}, []string{""})
	if got := flattenFields(shallow, []string{tagKeyJSON}); got["deep"] != "string" {
		t.Errorf("flattenFields of a one-level embed = %v, want the deep field", got)
	}
}

// TestDiffPair_URLTagNotation_MatchesTheSnakeCaseMCPName verifies the input
// diff's fallback tag match: an SDK url tag written in array or negation
// notation (iids[], not[author_id]) is matched to the snake_case MCP json name
// rather than reported missing, while a tag with no counterpart still is.
func TestDiffPair_URLTagNotation_MatchesTheSnakeCaseMCPName(t *testing.T) {
	mcp := makeStruct(
		structField{name: "IIDs", jsonTag: "iids", goType: types.NewSlice(tInt)},
		structField{name: "NotAuthorID", jsonTag: "not_author_id", goType: tInt},
	)
	sdk := makeStructWithTags(
		taggedField{name: "IIDs", tag: `url:"iids[]"`, goType: types.NewSlice(tInt)},
		taggedField{name: "NotAuthorID", tag: `url:"not[author_id]"`, goType: tInt},
		taggedField{name: "Scope", tag: `url:"scope"`, goType: tString},
	)

	g := diffPair("issues", "input", structPair{
		mcpName: "ListInput", mcpType: mcp,
		sdkName: "v2.ListIssuesOptions", sdkType: sdk, sdkURLTags: true,
	})
	if len(g.MissingFields) != 1 || g.MissingFields[0].Tag != "scope" {
		t.Errorf("missing fields = %+v, want only the unmatched scope tag", g.MissingFields)
	}
	if len(g.TypeMismatches) != 0 {
		t.Errorf("type mismatches = %+v, want none (the notation match keeps the types)", g.TypeMismatches)
	}
}

// TestSummarize_Reports_CountPackagesWithGaps verifies the report summary
// counts a package as gapped when any of the three gap classes is non-zero,
// sums the per-class totals, and tallies the advisory type mismatches.
func TestSummarize_Reports_CountPackagesWithGaps(t *testing.T) {
	s := summarize([]packageReport{
		{Package: "clean", InputPairs: 4, OutputPairs: 2},
		{Package: "missing_input", InputPairs: 1, MissingInputCount: 3},
		{Package: "missing_output", OutputPairs: 1, MissingOutputCount: 2},
		{Package: "extra_output", OutputPairs: 1, ExtraOutputCount: 1, Gaps: []gap{
			{Kind: "output", TypeMismatches: []typeMismatch{{Tag: "id"}, {Tag: "iid"}}},
		}},
	})
	want := reportSummary{
		Packages: 4, PackagesWithGaps: 3, InputPairs: 5, OutputPairs: 4,
		MissingInputFields: 3, MissingOutputFields: 2, ExtraOutputFields: 1, TypeMismatches: 2,
	}
	if s != want {
		t.Errorf("summarize = %+v, want %+v", s, want)
	}
}

// TestRun_Roots_EmitsJSONOrLoadError verifies the command-facing entry point:
// a root the loader cannot enter fails the run, and the repository root yields
// the report as indented JSON naming the SDK path, with gaps-only keeping only
// the packages that carry a finding.
func TestRun_Roots_EmitsJSONOrLoadError(t *testing.T) {
	if _, err := Run(filepath.Join(t.TempDir(), "absent"), true); err == nil || !strings.Contains(err.Error(), "load packages") {
		t.Fatalf("Run on a missing root = %v, want a load error", err)
	}

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("report lacks the trailing newline")
	}
	var rep report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.SchemaVersion != shared.SchemaVersion || rep.ClientGoPath != shared.ClientGoPkgPath {
		t.Errorf("report header = %d/%q, want %d/%q", rep.SchemaVersion, rep.ClientGoPath, shared.SchemaVersion, shared.ClientGoPkgPath)
	}
	for _, pr := range rep.Packages {
		if pr.MissingInputCount == 0 && pr.MissingOutputCount == 0 && pr.ExtraOutputCount == 0 {
			t.Errorf("gaps-only report kept %s, which has no gap", pr.Package)
		}
	}
}
