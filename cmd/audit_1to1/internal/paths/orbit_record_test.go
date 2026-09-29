package paths

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// orbitFixture is a package shaped like internal/tools/orbit where it matters:
// a status output publishing the flat copies client-go promotes beside the
// nested object GitLab sends, a nested type reached at two depths, a value
// passed through as any, a bare-array wrapper, a body carried whole, a type
// that reaches itself, and a field of a type from a package the kinds question
// does not model.
const orbitFixture = `package orbitx

import "example.com/toolutil"

type StatusOutput struct {
	toolutil.HintableOutput
	User       *StatusUser   ` + "`json:\"user,omitempty\"`" + `
	System     *StatusSystem ` + "`json:\"system,omitempty\"`" + `
	Status     string        ` + "`json:\"status,omitempty\"`" + `
	Components []Component   ` + "`json:\"components,omitempty\"`" + `
}

type StatusUser struct {
	Available bool ` + "`json:\"available\"`" + `
}

type StatusSystem struct {
	Status     string      ` + "`json:\"status,omitempty\"`" + `
	Error      string      ` + "`json:\"error,omitempty\"`" + `
	Components []Component ` + "`json:\"components,omitempty\"`" + `
	Owner      toolutil.Person ` + "`json:\"owner,omitempty\"`" + `
}

type Component struct {
	Name    string ` + "`json:\"name\"`" + `
	Metrics any    ` + "`json:\"metrics,omitempty\"`" + `
}

type ToolsOutput struct {
	toolutil.HintableOutput
	Tools []Tool ` + "`json:\"tools\"`" + `
}

type Tool struct {
	Name       string ` + "`json:\"name\"`" + `
	Parameters any    ` + "`json:\"parameters,omitempty\"`" + `
}

type PairOutput struct {
	Left  []Tool ` + "`json:\"left\"`" + `
	Right []Tool ` + "`json:\"right\"`" + `
}

type DSLOutput struct {
	Content        string ` + "`json:\"content\"`" + `
	ResponseFormat string ` + "`json:\"response_format\"`" + `
	Result         *Hidden ` + "`json:\"result,omitempty\"`" + `
}

type Hidden struct {
	Inside string ` + "`json:\"inside\"`" + `
}

type TreeOutput struct {
	Name     string       ` + "`json:\"name\"`" + `
	Children []TreeOutput ` + "`json:\"children\"`" + `
}

type ClockOutput struct {
	Uptime time.Duration ` + "`json:\"uptime\"`" + `
}
`

// orbitShared is the shared shapes package of the fixture tree: a person the
// status system names, and the hints type, whose fields are never published.
const orbitShared = `package toolutil

type HintableOutput struct {
	NextSteps []string ` + "`json:\"next_steps,omitempty\"`" + `
}

type Person struct {
	Login string ` + "`json:\"login\"`" + `
}
`

// keys spells a recorded key tree: a path ending in "!" kept verbatim, a path
// written "path=kind|kind" carrying those kinds, and every other path an
// object.
func keys(entries ...string) []orbitrecord.Key {
	var out []orbitrecord.Key
	for _, entry := range entries {
		spec, verbatim := strings.CutSuffix(entry, "!")
		path, kinds, spelled := strings.Cut(spec, "=")
		if !spelled {
			kinds = orbitrecord.KindObject
		}
		out = append(out, orbitrecord.Key{Path: path, Kinds: strings.Split(kinds, "|"), Verbatim: verbatim})
	}
	return out
}

// orbitCall is one recorded call of the fixture package.
func orbitCall(action, variant, output string, keyTree []orbitrecord.Key) orbitrecord.Call {
	return orbitrecord.Call{Action: action, Variant: variant, Output: "internal/tools/orbitx." + output, Response: orbitrecord.Response{Status: 200, Keys: keyTree}}
}

// stubOrbitRecord hands the check a record without writing one, empties the
// declaration table, whose real entries are about the real tree, and hands
// every check that asks for the converter pairing one naming no client-go
// directory, so the decoder half is skipped unless a case gives it one with
// stubDecoders.
func stubOrbitRecord(t *testing.T, doc orbitrecord.Document, err error) {
	t.Helper()
	previousRead, previousDeclarations, previousPairings := readOrbitRecord, declaredOrbitFields, collectPairings
	readOrbitRecord = func(string) (orbitrecord.Document, error) { return doc, err }
	declaredOrbitFields = nil
	collectPairings = func(string) (structs.Pairings, error) { return structs.Pairings{}, nil }
	t.Cleanup(func() {
		readOrbitRecord, declaredOrbitFields, collectPairings = previousRead, previousDeclarations, previousPairings
	})
}

// orbitTree writes the fixture package and its shared shapes under a new root.
func orbitTree(t *testing.T) string {
	t.Helper()
	root := writePackage(t, "orbitx", orbitFixture)
	writeShared(t, root, orbitShared)
	return root
}

// TestOrbitCheck_WithoutARecord_DoesNotRun verifies the one way the check is
// skipped, and that a skipped check says nothing stale and judges nothing.
func TestOrbitCheck_WithoutARecord_DoesNotRun(t *testing.T) {
	stubOrbitRecord(t, orbitrecord.Document{}, errors.New("no record"))

	check := orbitCheck(orbitTree(t))

	if !reflect.DeepEqual(check, OrbitCheck{}) || check.staleDeclarations() != nil || len(check.judged()) != 0 {
		t.Errorf("check = %+v, want nothing run", check)
	}
}

// TestOrbitCheck_AnOutputAndItsAnswers_AreComparedAtEveryDepth verifies both
// directions on the status shape: a flat copy GitLab never sends is reported
// once at its top and never for its fields, a field the nested object lacks is
// reported under it, a key an answer adds is reported at its top with the type
// it belongs in and the kinds of the key itself, never those of the elements
// of the list it holds, and nothing below a value published as any is judged.
func TestOrbitCheck_AnOutputAndItsAnswers_AreComparedAtEveryDepth(t *testing.T) {
	stubOrbitRecord(t, orbitrecord.Document{Source: orbitrecord.Source{OrbitVersion: "0.130.0"}, Calls: []orbitrecord.Call{
		orbitCall("orbit.status", "raw", "StatusOutput", keys(
			orbitrecord.Root, "user", "user.available=boolean", "user.extra", "user.extra.deep",
			"system", "system.status=string", "system.region", "system.owner", "system.owner.login=string",
			"system.components=array", "system.components[]", "system.components[].name=string",
			"system.components[].metrics", "system.components[].metrics.kind", "added=array", "added[]",
		)),
	}}, nil)

	check := orbitCheck(orbitTree(t))

	calls := []string{"orbit.status (raw)"}
	wantUnpublished := []OrbitField{
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "components", Type: "StatusOutput", Field: "components", Calls: calls},
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "status", Type: "StatusOutput", Field: "status", Calls: calls},
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "system.error", Type: "StatusSystem", Field: "error", Calls: calls},
	}
	object := []string{orbitrecord.KindObject}
	wantUnsurfaced := []OrbitField{
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "added", Type: "StatusOutput", Field: "added", Kinds: []string{orbitrecord.KindArray}, Calls: calls},
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "system.region", Type: "StatusSystem", Field: "region", Kinds: object, Calls: calls},
		{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "user.extra", Type: "StatusUser", Field: "extra", Kinds: object, Calls: calls},
	}
	if !reflect.DeepEqual(check.Unpublished, wantUnpublished) {
		t.Errorf("unpublished =\n%+v\nwant\n%+v", check.Unpublished, wantUnpublished)
	}
	if !reflect.DeepEqual(check.Unsurfaced, wantUnsurfaced) {
		t.Errorf("unsurfaced =\n%+v\nwant\n%+v", check.Unsurfaced, wantUnsurfaced)
	}
	if !slices.Equal(check.Compared, []string{"orbitx.StatusOutput"}) ||
		!slices.Equal(check.Nested, []string{"orbitx.Component", "orbitx.StatusSystem", "orbitx.StatusUser", "toolutil.Person"}) {
		t.Errorf("compared = %q, nested = %q", check.Compared, check.Nested)
	}
	if !check.Ran || check.Calls != 1 || check.Record != orbitrecord.FileName || check.Source.OrbitVersion != "0.130.0" {
		t.Errorf("check = %+v", check)
	}
	if check.undeclared() != 6 {
		t.Errorf("undeclared() = %d, want 6", check.undeclared())
	}
	// Every kind the answers carried is one its field decodes, so the third
	// question finds nothing where the first two found six.
	if check.Mismatched != nil || check.KindsUnjudged != nil {
		t.Errorf("mismatched = %+v, unjudged = %+v, want none", check.Mismatched, check.KindsUnjudged)
	}
}

// TestOrbitCheck_ABareArrayAnswer_IsItsWrappersOneField verifies the tools
// shape: an answer that is an array is compared with the one field of a type
// that wraps it, the wrapper key is not reported, and a type with two fields
// is not taken for a wrapper.
func TestOrbitCheck_ABareArrayAnswer_IsItsWrappersOneField(t *testing.T) {
	arrayRoot := keys("[]", "[].name=string", "[].parameters!")
	arrayRoot = append([]orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindArray}}}, arrayRoot...)
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.tools", "default", "ToolsOutput", arrayRoot),
		orbitCall("orbit.pairs", "default", "PairOutput", arrayRoot),
	}}, nil)

	check := orbitCheck(orbitTree(t))

	if !slices.Equal(check.Wrappers, []string{"orbitx.ToolsOutput"}) {
		t.Errorf("wrappers = %q", check.Wrappers)
	}
	var reported []string
	for _, finding := range check.Unpublished {
		reported = append(reported, finding.Output+" "+finding.Path)
	}
	if !slices.Equal(reported, []string{"PairOutput left", "PairOutput right"}) {
		t.Errorf("unpublished = %q, want the two fields of the type that is not a wrapper", reported)
	}
	var sent []string
	for _, finding := range check.Unsurfaced {
		sent = append(sent, finding.Output+" "+finding.Path)
	}
	if !slices.Equal(sent, []string{"PairOutput name", "PairOutput parameters"}) {
		t.Errorf("unsurfaced = %q, want the array's keys held against the pair and nothing against the wrapper", sent)
	}
}

// TestOrbitCheck_AVerbatimBody_ShieldsNothingOfItsCarrier verifies the DSL
// shape: a body kept verbatim has no keys, so the type carrying it has every
// field reported (a declaration says it is this server's framing), while a
// verbatim subtree below the body shields what a type models inside it.
func TestOrbitCheck_AVerbatimBody_ShieldsNothingOfItsCarrier(t *testing.T) {
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.dsl", "raw", "DSLOutput", keys("$!")),
		orbitCall("orbit.dsl", "hidden", "DSLOutput", keys(orbitrecord.Root, "result!")),
	}}, nil)

	check := orbitCheck(orbitTree(t))

	var reported []string
	for _, finding := range check.Unpublished {
		reported = append(reported, finding.Path)
		if !slices.Equal(finding.Calls, []string{"orbit.dsl (raw)", "orbit.dsl (hidden)"}) {
			t.Errorf("calls = %q, want both calls of the output", finding.Calls)
		}
	}
	if !slices.Equal(reported, []string{"content", "response_format"}) {
		t.Errorf("unpublished = %q, want the carrier's fields and nothing below the verbatim result", reported)
	}
	if check.Unsurfaced != nil {
		t.Errorf("unsurfaced = %+v, want none", check.Unsurfaced)
	}
}

// TestOrbitCheck_ATypeThatReachesItself_IsWalkedOnce verifies the walk ends on
// a recursive type, publishing the recursive field as a leaf so what the
// answer carries below it is passed through rather than judged.
func TestOrbitCheck_ATypeThatReachesItself_IsWalkedOnce(t *testing.T) {
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.tree", "raw", "TreeOutput", keys(orbitrecord.Root, "name=string", "children=array", "children[]", "children[].name=string", "children[].children=array")),
	}}, nil)

	check := orbitCheck(orbitTree(t))

	if check.Unpublished != nil || check.Unsurfaced != nil || check.Nested != nil {
		t.Errorf("check = %+v, want the recursive type compared once and cleanly", check)
	}
}

// TestOrbitCheck_ARecordedTypeTheTreeLacks_IsStale verifies a record naming a
// type the tree no longer declares, or no package at all, is reported through
// the stale list, which is what makes the gate fail until it is re-recorded.
func TestOrbitCheck_ARecordedTypeTheTreeLacks_IsStale(t *testing.T) {
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.gone", "raw", "GoneOutput", keys(orbitrecord.Root)),
		{Action: "orbit.bare", Variant: "raw", Output: "BareOutput"},
	}}, nil)

	check := orbitCheck(orbitTree(t))

	if !slices.Equal(check.Missing, []string{"BareOutput", "internal/tools/orbitx.GoneOutput"}) || check.Compared != nil {
		t.Errorf("missing = %q, compared = %q", check.Missing, check.Compared)
	}
	want := []string{
		"BareOutput is named by the Orbit response record and the tree declares no such type: re-record with make gen-orbit-record",
		"internal/tools/orbitx.GoneOutput is named by the Orbit response record and the tree declares no such type: re-record with make gen-orbit-record",
	}
	if got := check.staleDeclarations(); !slices.Equal(got, want) {
		t.Errorf("staleDeclarations() = %q, want %q", got, want)
	}
}

// TestOrbitCheck_Judged_NamesEveryComparedType verifies the set the type
// grain's skip lists are cleared by: output and nested types alike.
func TestOrbitCheck_Judged_NamesEveryComparedType(t *testing.T) {
	check := OrbitCheck{Compared: []string{"orbit.A"}, Nested: []string{"orbit.B"}}
	if got := check.judged(); !reflect.DeepEqual(got, map[string]bool{"orbit.A": true, "orbit.B": true}) {
		t.Errorf("judged() = %v", got)
	}
}

// TestOrbitHelpers_PathsAndOrder verifies the small helpers the walk and the
// report lean on.
func TestOrbitHelpers_PathsAndOrder(t *testing.T) {
	if joinPath("", "a") != "a" || joinPath("a", "") != "a" || joinPath("a", "b") != "a.b" {
		t.Error("joinPath() drops or doubles a segment")
	}
	if parentPath("a.b.c") != "a.b" || parentPath("a") != "" {
		t.Error("parentPath() walks up wrongly")
	}
	if lastSegment("a.b.c") != "c" || lastSegment("a") != "a" {
		t.Error("lastSegment() reads the wrong key")
	}
	if pkg, name := splitOutput("internal/tools/orbit.StatusOutput"); pkg != "internal/tools/orbit" || name != "StatusOutput" {
		t.Errorf("splitOutput() = %q, %q", pkg, name)
	}
	// A leading dot is a package that is empty, not part of the type's name.
	if pkg, name := splitOutput(".StatusOutput"); pkg != "" || name != "StatusOutput" {
		t.Errorf("splitOutput(.StatusOutput) = %q, %q", pkg, name)
	}
	if bareArray(nil) {
		t.Error("bareArray(nil) = true, want false: no answer is no array")
	}
	mixed := []orbitrecord.Call{{Response: orbitrecord.Response{Keys: []orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindArray, orbitrecord.KindNull}}}}}}
	if bareArray(mixed) {
		t.Error("bareArray() = true for a body that was sometimes null")
	}
	found := []OrbitField{
		{Package: "b", Output: "A", Path: "a"},
		{Package: "a", Output: "B", Path: "a"},
		{Package: "a", Output: "A", Path: "b"},
		{Package: "a", Output: "A", Path: "a"},
	}
	sortOrbitFindings(found)
	var order []string
	for _, finding := range found {
		order = append(order, finding.Package+finding.Output+finding.Path)
	}
	if !slices.Equal(order, []string{"aAa", "aAb", "aBa", "bAa"}) {
		t.Errorf("order = %q", order)
	}
	// Sorted input asks every comparison the other way round, which is what
	// holds each tie-breaker to both of its answers.
	sorted := []OrbitField{{Package: "a", Output: "A", Path: "a"}, {Package: "a", Output: "A", Path: "b"}, {Package: "a", Output: "B", Path: "a"}, {Package: "b", Output: "A", Path: "a"}}
	sortOrbitFindings(sorted)
	if sorted[0].Path != "a" || sorted[1].Path != "b" || sorted[2].Output != "B" || sorted[3].Package != "b" {
		t.Errorf("a sorted list was reordered: %+v", sorted)
	}
}

// TestTypedShapeCheck_JudgedAgainstOrbit_LeavesTheSkipListsToWhatNobodyJudged
// verifies the type grain hands the Orbit types to the record: each is taken
// out of whichever skip list it fell into, the counters follow the lists, and
// the moved names are listed once, sorted.
func TestTypedShapeCheck_JudgedAgainstOrbit_LeavesTheSkipListsToWhatNobodyJudged(t *testing.T) {
	check := TypedShapeCheck{
		SkippedNoPairing: 2, SkippedNoRoute: 1, SkippedNoSchema: 2,
		Skipped: SkippedTypes{
			NoPairing: []string{"issues.ListOutput", "orbit.DSLOutput"},
			NoRoute:   []string{"orbit.ToolDefinition"},
			NoSchema:  []string{"geo.Other", "orbit.StatusOutput"},
		},
	}
	check = check.judgedAgainstOrbit(map[string]bool{"orbit.DSLOutput": true, "orbit.ToolDefinition": true, "orbit.StatusOutput": true, "orbit.Nested": true})

	want := SkippedTypes{NoPairing: []string{"issues.ListOutput"}, NoSchema: []string{"geo.Other"}}
	if !reflect.DeepEqual(check.Skipped, want) || check.SkippedNoPairing != 1 || check.SkippedNoRoute != 0 || check.SkippedNoSchema != 1 {
		t.Errorf("skipped = %+v (%d, %d, %d)", check.Skipped, check.SkippedNoPairing, check.SkippedNoRoute, check.SkippedNoSchema)
	}
	if !slices.Equal(check.OrbitRecord, []string{"orbit.DSLOutput", "orbit.StatusOutput", "orbit.ToolDefinition"}) {
		t.Errorf("OrbitRecord = %q", check.OrbitRecord)
	}
}
