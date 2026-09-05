// Package enums tests the enum value rule (R-ENUM): that every field of a
// client-go enum type an action exposes offers exactly the values the SDK
// declares. The end-to-end cases run against a throwaway module carrying a
// stand-in client-go, so a case can state the whole universe it asserts
// about instead of depending on the repository's shape.
package enums

import (
	"encoding/json"
	"errors"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fixtureSDK is a stand-in for the client-go root package. Its import path
// carries the client-go path as an infix, which is what the resolver matches
// on, so the analyzer treats it exactly as it treats the real SDK. It declares
// one string enum with an aliased value, one integer enum, a float named type
// (not an enum kind), and a named string with no constants (not an enum).
const fixtureSDK = `package gitlab

// RequestOptionFunc is the variadic tail that marks a REST endpoint.
type RequestOptionFunc func()

// ColorValue is a string enum; Azure repeats Blue's value.
type ColorValue string

const (
	Red   ColorValue = "red"
	Blue  ColorValue = "blue"
	Azure ColorValue = "blue"
)

// SizeValue is an integer enum.
type SizeValue int

const (
	Small SizeValue = 1
	Large SizeValue = 3
)

// WeightValue has constants but is not a string or integer kind.
type WeightValue float64

const Heavy WeightValue = 2.5

// PlainID is a named string with no constants, so it is not an enum.
type PlainID string

type ListWidgetsOptions struct {
	Color  *ColorValue ` + "`url:\"color,omitempty\" json:\"color,omitempty\"`" + `
	Sizes  []SizeValue ` + "`url:\"sizes[],omitempty\" json:\"sizes,omitempty\"`" + `
	Shade  ColorValue  ` + "`url:\"shade,omitempty\" json:\"shade,omitempty\"`" + `
	ID     PlainID     ` + "`url:\"id,omitempty\" json:\"id,omitempty\"`" + `
	Weight WeightValue ` + "`url:\"weight,omitempty\" json:\"weight,omitempty\"`" + `
	Name   string      ` + "`url:\"name,omitempty\" json:\"name,omitempty\"`" + `
	hidden ColorValue
}

type Widget struct {
	Color ColorValue ` + "`json:\"color\"`" + `
	Size  SizeValue  ` + "`json:\"size\"`" + `
	Name  string     ` + "`json:\"name\"`" + `
}

// WidgetDetail is a second result shape the same MCP output is built from,
// which is how one MCP tag meets the same enum through two SDK structs.
type WidgetDetail struct {
	Color ColorValue ` + "`json:\"color\"`" + `
	Name  string     ` + "`json:\"name\"`" + `
}

type WidgetsServiceInterface interface {
	ListWidgets(opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, error)
}

type Client struct {
	Widgets WidgetsServiceInterface
}
`

// fixtureTool is a tool package whose handler pairs ListInput with the SDK
// options and whose converter pairs Output with the SDK result. shade has no
// MCP counterpart on purpose: an SDK enum field the input does not expose is
// the struct rule's finding, not this one's.
const fixtureTool = `package widgets

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v2"

type ListInput struct {
	Color string ` + "`json:\"color\"`" + `
	Sizes []int  ` + "`json:\"sizes\"`" + `
	Name  string ` + "`json:\"name\"`" + `
}

type Output struct {
	Color string ` + "`json:\"color\"`" + `
	Size  int    ` + "`json:\"size\"`" + `
	Name  string ` + "`json:\"name\"`" + `
}

// List builds the SDK options from the input, which is the pairing the rule reads.
func List(c *gl.Client, input ListInput) ([]Output, error) {
	opts := &gl.ListWidgetsOptions{Name: input.Name}
	items, err := c.Widgets.ListWidgets(opts)
	if err != nil {
		return nil, err
	}
	out := make([]Output, 0, len(items))
	for _, item := range items {
		out = append(out, toOutput(item))
	}
	return out, nil
}

// toOutput is the converter that pairs Output with the SDK result.
func toOutput(w *gl.Widget) Output {
	return Output{Color: string(w.Color), Size: int(w.Size), Name: w.Name}
}

// fromDetail pairs the same Output with a second SDK struct.
func fromDetail(w *gl.WidgetDetail) Output {
	return Output{Color: string(w.Color), Name: w.Name}
}
`

const (
	fixtureModulePath = "example.com/fixture"
	fixtureSDKFile    = "gitlab.com/gitlab-org/api/client-go/v2/sdk.go"
	fixtureInputKey   = fixtureModulePath + "/internal/tools/widgets.ListInput"
	fixtureOutputKey  = fixtureModulePath + "/internal/tools/widgets.Output"
	fixtureAction     = "widget.list"
)

// fixtureModule writes the fixture module and returns its root. Extra files
// are merged in, overriding the defaults.
func fixtureModule(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"go.mod":                            "module " + fixtureModulePath + "\n\ngo 1.27\n",
		fixtureSDKFile:                      fixtureSDK,
		"internal/tools/widgets/widgets.go": fixtureTool,
	}
	maps.Copy(files, extra)
	root := t.TempDir()
	writeFiles(t, root, files)
	return root
}

// writeFiles writes each slash-separated relative path under root, creating
// parents as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// loadFixture loads the fixture module's tool packages and its stand-in SDK.
func loadFixture(t *testing.T, root string) ([]*packages.Package, *types.Package) {
	t.Helper()
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load fixture packages: %v", err)
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("client-go types: %v", err)
	}
	return pkgs, clientGo
}

// fixtureOffered is the catalog half of the rule for the fixture: what one
// action's schemas say about the two structs.
func fixtureOffered(input, output map[string]property) offeredIndex {
	return offeredIndex{
		fixtureInputKey:  {{Action: fixtureAction, Kind: kindInput, Properties: input}},
		fixtureOutputKey: {{Action: fixtureAction, Kind: kindOutput, Properties: output}},
	}
}

// findingsByField indexes a report's findings by "<kind> <field>".
func findingsByField(rep Report) map[string]Finding {
	out := map[string]Finding{}
	for _, pkg := range rep.Packages {
		for _, f := range pkg.Findings {
			out[f.Kind+" "+f.Field] = f
		}
	}
	return out
}

// TestCollectSDKEnums_Fixture_KeepsStringAndIntegerTypesWithConstants
// verifies the SDK half of the rule: a string or integer named type with
// constants is an enum, two constants with one value are one value, and a
// float kind or a constant-less named string is left out.
func TestCollectSDKEnums_Fixture_KeepsStringAndIntegerTypesWithConstants(t *testing.T) {
	_, clientGo := loadFixture(t, fixtureModule(t, nil))
	got := collectSDKEnums(clientGo)
	want := map[string]sdkEnum{
		"ColorValue": {Name: "ColorValue", Values: []string{"blue", "red"}},
		"SizeValue":  {Name: "SizeValue", Values: []string{"1", "3"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectSDKEnums = %+v, want %+v", got, want)
	}
}

// TestCollectExposedFields_Fixture_PairsEnumFieldsWithTheirMCPTag verifies
// the AST half: the enum fields of both pairs are found under the MCP tag,
// the array notation on sizes[] resolves to sizes, an output tag reached
// through two converters (Widget and WidgetDetail) is held once, and shade
// (no MCP counterpart), id (no constants), weight (float) and name (string)
// are not enum fields.
func TestCollectExposedFields_Fixture_PairsEnumFieldsWithTheirMCPTag(t *testing.T) {
	pkgs, clientGo := loadFixture(t, fixtureModule(t, nil))
	got := collectExposedFields(pkgs, collectSDKEnums(clientGo))
	pkgPath := fixtureModulePath + "/internal/tools/widgets"
	want := []exposedField{
		{PkgPath: pkgPath, Package: "widgets", Kind: kindInput, MCPType: "ListInput", Tag: "color", SDKType: "v2.ListWidgetsOptions", SDKField: "Color", Enum: "ColorValue"},
		{PkgPath: pkgPath, Package: "widgets", Kind: kindInput, MCPType: "ListInput", Tag: "sizes", SDKType: "v2.ListWidgetsOptions", SDKField: "Sizes", Enum: "SizeValue"},
		{PkgPath: pkgPath, Package: "widgets", Kind: kindOutput, MCPType: "Output", Tag: "color", SDKType: "v2.Widget", SDKField: "Color", Enum: "ColorValue"},
		{PkgPath: pkgPath, Package: "widgets", Kind: kindOutput, MCPType: "Output", Tag: "size", SDKType: "v2.Widget", SDKField: "Size", Enum: "SizeValue"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectExposedFields = %+v, want %+v", got, want)
	}
	if key := got[0].structKey(); key != fixtureInputKey {
		t.Errorf("structKey = %q, want %q", key, fixtureInputKey)
	}
}

// TestCollectExposedFields_Fixture_AnAliasedOutputIsKeyedByItsTarget pins the
// identity an alias resolves to. A converter returning
// `type Output = shapes.WidgetOutput` reflects as shapes.WidgetOutput, which is
// what the catalog index keys the schema on; an exposed field keyed by the
// alias name would never meet that offer, and the field would fall out of the
// rule without a finding, which is how every aliased output was skipped before.
func TestCollectExposedFields_Fixture_AnAliasedOutputIsKeyedByItsTarget(t *testing.T) {
	root := fixtureModule(t, map[string]string{
		"internal/shapes/shapes.go": `package shapes

type WidgetOutput struct {
	Color string ` + "`json:\"color\"`" + `
}
`,
		"internal/tools/gadgets/gadgets.go": `package gadgets

import (
	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v2"

	"example.com/fixture/internal/shapes"
)

type Output = shapes.WidgetOutput

// toOutput pairs the aliased Output with the SDK result.
func toOutput(w *gl.Widget) Output {
	return Output{Color: string(w.Color)}
}
`,
	})
	pkgs, clientGo := loadFixture(t, root)
	var got []exposedField
	for _, field := range collectExposedFields(pkgs, collectSDKEnums(clientGo)) {
		if field.Package == "gadgets" {
			got = append(got, field)
		}
	}
	targetPath := fixtureModulePath + "/internal/shapes"
	want := []exposedField{
		{PkgPath: targetPath, Package: "gadgets", Kind: kindOutput, MCPType: "WidgetOutput", Tag: "color", SDKType: "v2.Widget", SDKField: "Color", Enum: "ColorValue"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectExposedFields(gadgets) = %+v, want %+v", got, want)
	}
	if key := got[0].structKey(); key != targetPath+".WidgetOutput" {
		t.Errorf("structKey = %q, want the alias target %q", key, targetPath+".WidgetOutput")
	}
}

// TestBuildReport_Fixture_HoldsEachFieldToTheSDKValues runs the whole rule on
// the fixture: an enum-sourced input with one missing and one extra value, a
// clean enum-sourced list input, a description-sourced output that names
// every value, and an output that surfaces nothing and is counted as
// unsurfaced rather than reported.
func TestBuildReport_Fixture_HoldsEachFieldToTheSDKValues(t *testing.T) {
	pkgs, clientGo := loadFixture(t, fixtureModule(t, nil))
	offered := fixtureOffered(
		map[string]property{
			"color": {Enum: []string{"blue", "green"}},
			"sizes": {Enum: []string{"1", "3"}},
		},
		map[string]property{
			"color": {Description: "The color, red or blue."},
			"size":  {},
		},
	)
	rep := buildReport(pkgs, collectSDKEnums(clientGo), offered, nil, false)

	if rep.Summary != (Summary{Packages: 1, SDKEnums: 2, Fields: 4, FieldsWithGaps: 1, UnsurfacedOutputFields: 1, MissingValues: 1, ExtraValues: 1}) {
		t.Errorf("summary = %+v", rep.Summary)
	}
	byField := findingsByField(rep)
	cases := []struct {
		name string
		want Finding
	}{
		{name: "input color", want: Finding{
			Action: fixtureAction, Kind: kindInput, MCPType: "ListInput", Field: "color",
			SDKType: "v2.ListWidgetsOptions", SDKField: "Color", Enum: "ColorValue", Source: sourceEnum,
			SDKValues: []string{"blue", "red"}, Offered: []string{"blue", "green"},
			Missing: []string{"red"}, Extra: []string{"green"},
		}},
		{name: "input sizes", want: Finding{
			Action: fixtureAction, Kind: kindInput, MCPType: "ListInput", Field: "sizes",
			SDKType: "v2.ListWidgetsOptions", SDKField: "Sizes", Enum: "SizeValue", Source: sourceEnum,
			SDKValues: []string{"1", "3"}, Offered: []string{"1", "3"},
		}},
		{name: "output color", want: Finding{
			Action: fixtureAction, Kind: kindOutput, MCPType: "Output", Field: "color",
			SDKType: "v2.Widget", SDKField: "Color", Enum: "ColorValue", Source: sourceDescription,
			SDKValues: []string{"blue", "red"}, Offered: []string{"blue", "red"},
		}},
		{name: "output size", want: Finding{
			Action: fixtureAction, Kind: kindOutput, MCPType: "Output", Field: "size",
			SDKType: "v2.Widget", SDKField: "Size", Enum: "SizeValue", Source: sourceNone,
			SDKValues: []string{"1", "3"}, Offered: []string{},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := byField[tc.name]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("finding = %+v, want %+v", got, tc.want)
			}
		})
	}
	if rep.Summary.Clean() {
		t.Error("a report with a value gap reported a clean gate")
	}

	t.Run("gaps_only_keeps_the_one_field", func(t *testing.T) {
		gaps := buildReport(pkgs, collectSDKEnums(clientGo), offered, nil, true)
		if len(gaps.Packages) != 1 || len(gaps.Packages[0].Findings) != 1 || gaps.Packages[0].Findings[0].Field != "color" {
			t.Errorf("gaps-only packages = %+v, want only the color input", gaps.Packages)
		}
		if gaps.Summary != rep.Summary {
			t.Errorf("gaps-only summary = %+v, want the full report's %+v", gaps.Summary, rep.Summary)
		}
	})

	t.Run("gaps_only_drops_a_clean_package", func(t *testing.T) {
		clean := fixtureOffered(map[string]property{"color": {Enum: []string{"blue", "red"}}, "sizes": {Enum: []string{"1", "3"}}}, nil)
		gaps := buildReport(pkgs, collectSDKEnums(clientGo), clean, nil, true)
		if len(gaps.Packages) != 0 || !gaps.Summary.Clean() {
			t.Errorf("gaps-only report of a clean package = %+v, want no packages and a clean gate", gaps)
		}
	})
}

// TestBuildReport_Fixture_InputWithNoValueSetIsMissingEverything verifies
// the asymmetry between the two sides: an input whose schema neither
// enumerates nor describes the values is a gap on every SDK value, while an
// input the schema does not carry at all (a pruned property) is treated the
// same, and an offer of the other kind is not read against the field.
func TestBuildReport_Fixture_InputWithNoValueSetIsMissingEverything(t *testing.T) {
	pkgs, clientGo := loadFixture(t, fixtureModule(t, nil))
	offered := offeredIndex{
		fixtureInputKey: {
			{Action: fixtureAction, Kind: kindInput, Properties: map[string]property{"color": {Description: "pick one"}}},
			{Action: "widget.other", Kind: kindOutput, Properties: map[string]property{"color": {Enum: []string{"blue", "red"}}}},
		},
	}
	rep := buildReport(pkgs, collectSDKEnums(clientGo), offered, nil, true)
	byField := findingsByField(rep)
	color, ok := byField["input color"]
	if !ok || color.Source != sourceNone || !reflect.DeepEqual(color.Missing, []string{"blue", "red"}) || color.Extra != nil {
		t.Errorf("color finding = %+v, want every SDK value missing and nothing extra", color)
	}
	sizes, ok := byField["input sizes"]
	if !ok || sizes.Source != sourceNone || !reflect.DeepEqual(sizes.Missing, []string{"1", "3"}) {
		t.Errorf("sizes finding = %+v, want every SDK value missing (property absent from the schema)", sizes)
	}
	if rep.Summary.Fields != 2 {
		t.Errorf("fields = %d, want the two input fields only (the output-kind offer is not read against them)", rep.Summary.Fields)
	}
}

// TestBuildReport_Fixture_ExemptionsExcuseAndGoStale verifies the exemption
// table in both key forms and its staleness rule: a field-level key excuses
// every gap on the field, a value-level key excuses one value, and a key that
// excuses nothing (a closed gap or an unexposed field) is reported stale.
func TestBuildReport_Fixture_ExemptionsExcuseAndGoStale(t *testing.T) {
	pkgs, clientGo := loadFixture(t, fixtureModule(t, nil))
	sdk := collectSDKEnums(clientGo)
	offered := fixtureOffered(map[string]property{
		"color": {Enum: []string{"blue", "green"}},
		"sizes": {Enum: []string{"1"}},
	}, nil)

	cases := []struct {
		name        string
		exemptions  map[string]string
		wantMissing map[string][]string
		wantExtra   map[string][]string
		wantReason  map[string]string
		wantStale   []string
		wantClean   bool
	}{
		{
			name:        "no_table",
			wantMissing: map[string][]string{"color": {"red"}, "sizes": {"3"}},
			wantExtra:   map[string][]string{"color": {"green"}},
		},
		{
			name:        "field_level_excuses_both_directions",
			exemptions:  map[string]string{"widgets.ListInput.color": "documented subset"},
			wantMissing: map[string][]string{"sizes": {"3"}},
			wantReason:  map[string]string{"color": "documented subset"},
		},
		{
			name:       "value_level_excuses_one_value_each",
			exemptions: map[string]string{"widgets.ListInput.color=red": "not accepted here", "widgets.ListInput.sizes=3": "too large", "widgets.ListInput.color=green": "documented, no constant upstream"},
			wantReason: map[string]string{"color": "documented, no constant upstream", "sizes": "too large"},
			wantClean:  true,
		},
		{
			name:       "stale_keys_fail_the_gate",
			exemptions: map[string]string{"widgets.ListInput.color": "subset", "widgets.ListInput.sizes=3": "too large", "widgets.ListInput.sizes=1": "offered, nothing to excuse", "widgets.ListInput.gone": "field no longer exists", "widgets.Output.size": "output surfaces nothing, so no gap"},
			wantReason: map[string]string{"color": "subset", "sizes": "too large"},
			wantStale:  []string{"enum widgets.ListInput.gone", "enum widgets.ListInput.sizes=1", "enum widgets.Output.size"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := buildReport(pkgs, sdk, offered, tc.exemptions, false)
			byField := findingsByField(rep)
			for _, field := range []string{"color", "sizes"} {
				f := byField["input "+field]
				if !reflect.DeepEqual(f.Missing, tc.wantMissing[field]) || !reflect.DeepEqual(f.Extra, tc.wantExtra[field]) {
					t.Errorf("%s missing/extra = %v/%v, want %v/%v", field, f.Missing, f.Extra, tc.wantMissing[field], tc.wantExtra[field])
				}
				if f.Exemption != tc.wantReason[field] {
					t.Errorf("%s exemption = %q, want %q", field, f.Exemption, tc.wantReason[field])
				}
			}
			if !reflect.DeepEqual(rep.StaleExemptions, tc.wantStale) {
				t.Errorf("stale = %v, want %v", rep.StaleExemptions, tc.wantStale)
			}
			if rep.Summary.StaleExemptions != len(tc.wantStale) || rep.Summary.Clean() != tc.wantClean {
				t.Errorf("summary = %+v, want %d stale and clean=%v", rep.Summary, len(tc.wantStale), tc.wantClean)
			}
		})
	}
}

// TestCompare_Sources_ReadTheOfferFromWhereItIs verifies the comparison in
// isolation over its four sources: an enum list is compared both ways, prose
// is compared one way, nothing surfaced is a gap on every value for an input,
// and an output with nothing surfaced is left alone.
func TestCompare_Sources_ReadTheOfferFromWhereItIs(t *testing.T) {
	sdk := sdkEnum{Name: "StateValue", Values: []string{"closed", "opened"}}
	input := exposedField{Kind: kindInput, MCPType: "In", Tag: "state", SDKType: "v2.Options", SDKField: "State", Enum: "StateValue"}
	output := exposedField{Kind: kindOutput, MCPType: "Out", Tag: "state", SDKType: "v2.Result", SDKField: "State", Enum: "StateValue"}
	cases := []struct {
		name        string
		field       exposedField
		prop        map[string]property
		wantSource  string
		wantOffered []string
		wantMissing []string
		wantExtra   []string
	}{
		{name: "enum_both_ways", field: input, prop: map[string]property{"state": {Enum: []string{"locked", "opened"}}}, wantSource: sourceEnum, wantOffered: []string{"locked", "opened"}, wantMissing: []string{"closed"}, wantExtra: []string{"locked"}},
		{name: "description_one_way", field: input, prop: map[string]property{"state": {Description: "opened, or locked"}}, wantSource: sourceDescription, wantOffered: []string{"opened"}, wantMissing: []string{"closed"}},
		{name: "description_naming_nothing", field: input, prop: map[string]property{"state": {Description: "the state"}}, wantSource: sourceNone, wantOffered: []string{}, wantMissing: []string{"closed", "opened"}},
		{name: "input_property_absent", field: input, prop: map[string]property{}, wantSource: sourceNone, wantOffered: []string{}, wantMissing: []string{"closed", "opened"}},
		{name: "output_unsurfaced", field: output, prop: map[string]property{"state": {}}, wantSource: sourceNone, wantOffered: []string{}},
		{name: "output_description_stale", field: output, prop: map[string]property{"state": {Description: "closed only"}}, wantSource: sourceDescription, wantOffered: []string{"closed"}, wantMissing: []string{"opened"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compare(tc.field, sdk, actionOffer{Action: "a.b", Kind: tc.field.Kind, Properties: tc.prop})
			if got.Source != tc.wantSource || !reflect.DeepEqual(got.Offered, tc.wantOffered) ||
				!reflect.DeepEqual(got.Missing, tc.wantMissing) || !reflect.DeepEqual(got.Extra, tc.wantExtra) {
				t.Errorf("compare = source %q offered %v missing %v extra %v, want %q/%v/%v/%v",
					got.Source, got.Offered, got.Missing, got.Extra, tc.wantSource, tc.wantOffered, tc.wantMissing, tc.wantExtra)
			}
			if got.Action != "a.b" || got.Enum != "StateValue" || !reflect.DeepEqual(got.SDKValues, sdk.Values) {
				t.Errorf("compare carried action %q enum %q values %v", got.Action, got.Enum, got.SDKValues)
			}
		})
	}
}

// TestMentionedValues_Prose_ReadsWholeTokensOutsideNegations verifies how a
// description is read: whole tokens, case-insensitively, integers included,
// with a sentence that exists to exclude values skipped.
func TestMentionedValues_Prose_ReadsWholeTokensOutsideNegations(t *testing.T) {
	cases := []struct {
		name        string
		description string
		values      []string
		want        []string
	}{
		{name: "listed", description: "Package ecosystem: cargo, composer, or npm", values: []string{"cargo", "composer", "gem", "npm"}, want: []string{"cargo", "composer", "npm"}},
		{name: "case_insensitive", description: "One of Enabled or DISABLED", values: []string{"disabled", "enabled", "private"}, want: []string{"disabled", "enabled"}},
		{name: "whole_tokens_only", description: "set npm_token before use", values: []string{"npm", "token"}, want: []string{}},
		{name: "integers", description: "Access level (0=No access 30=Developer 40=Maintainer)", values: []string{"0", "30", "40", "60"}, want: []string{"0", "30", "40"}},
		{name: "negated_sentence_skipped", description: "Access level (10=Guest, 20=Reporter). 5 and 60 are not valid for shares", values: []string{"5", "10", "20", "60"}, want: []string{"10", "20"}},
		{name: "negation_after_semicolon", description: "one of red or blue; green is invalid", values: []string{"blue", "green", "red"}, want: []string{"blue", "red"}},
		{name: "empty", description: "", values: []string{"a"}, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mentionedValues(tc.description, tc.values); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mentionedValues(%q) = %v, want %v", tc.description, got, tc.want)
			}
		})
	}
}

// TestSchemaProperties_Shapes_ReadEnumsItemsAndDescriptions verifies the
// schema reader: an enum on the property, an enum on an array's items, a
// description when there is no enum, integers rendered bare whether Go ints
// or JSON floats, a property that is not an object skipped, and a schema
// without properties yielding nothing.
func TestSchemaProperties_Shapes_ReadEnumsItemsAndDescriptions(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"state":  map[string]any{"type": "string", "enum": []any{"opened", "closed"}, "description": "ignored when an enum exists"},
			"levels": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "enum": []any{40, 30.0, 0}}},
			"name":   map[string]any{"type": "string", "description": "Free text"},
			"bare":   map[string]any{"type": "string"},
			"odd":    "not an object",
		},
	}
	want := map[string]property{
		"state":  {Enum: []string{"closed", "opened"}},
		"levels": {Enum: []string{"0", "30", "40"}},
		"name":   {Description: "Free text"},
		"bare":   {},
	}
	if got := schemaProperties(schema); !reflect.DeepEqual(got, want) {
		t.Errorf("schemaProperties = %+v, want %+v", got, want)
	}
	if got := schemaProperties(map[string]any{"type": "object"}); len(got) != 0 {
		t.Errorf("schemaProperties without properties = %+v, want none", got)
	}
	if got := schemaProperties(nil); len(got) != 0 {
		t.Errorf("schemaProperties(nil) = %+v, want none", got)
	}
}

// fixtureInput and fixtureOutput are the reflect types collectOffered is fed.
type (
	fixtureInput  struct{ State string }
	fixtureOutput struct{ State string }
)

// TestCollectOffered_Catalog_KeysSchemasByTheirReflectIdentity verifies the
// catalog reader: each action's input and output schema lands under the
// struct's import path and name, a pointer input type resolves to its
// element, and a route with no typed input or an unnamed one is skipped.
func TestCollectOffered_Catalog_KeysSchemasByTheirReflectIdentity(t *testing.T) {
	inputSchema := map[string]any{"properties": map[string]any{"state": map[string]any{"enum": []any{"opened"}}}}
	outputSchema := map[string]any{"properties": map[string]any{"state": map[string]any{"description": "opened or closed"}}}
	groups := []tools.ActionSpecGroup{{
		ToolName: "gitlab_widget",
		Actions: []toolutil.ActionSpec{
			{Name: "list", Route: toolutil.ActionRoute{InputType: reflect.TypeFor[*fixtureInput](), InputSchema: inputSchema, OutputType: reflect.TypeFor[fixtureOutput](), OutputSchema: outputSchema}},
			{Name: "get", Route: toolutil.ActionRoute{InputType: reflect.TypeFor[fixtureInput](), InputSchema: inputSchema, OutputType: reflect.TypeFor[map[string]any]()}},
			{Name: "untyped", Route: toolutil.ActionRoute{}},
		},
	}}
	got := collectOffered(groups)

	pkgPath := reflect.TypeFor[fixtureInput]().PkgPath()
	inputs := got[pkgPath+".fixtureInput"]
	if len(inputs) != 2 || inputs[0].Action != "widget.list" || inputs[1].Action != "widget.get" || inputs[0].Kind != kindInput {
		t.Errorf("input offers = %+v, want list and get, both inputs", inputs)
	}
	if !reflect.DeepEqual(inputs[0].Properties, map[string]property{"state": {Enum: []string{"opened"}}}) {
		t.Errorf("input properties = %+v", inputs[0].Properties)
	}
	outputs := got[pkgPath+".fixtureOutput"]
	if len(outputs) != 1 || outputs[0].Kind != kindOutput || outputs[0].Properties["state"].Description != "opened or closed" {
		t.Errorf("output offers = %+v, want one output offer with the description", outputs)
	}
	if len(got) != 2 {
		t.Errorf("index has %d keys, want 2 (the map output and the untyped route are skipped)", len(got))
	}
}

// TestEnumTypeOf_Types_UnwrapsPointersAndSlicesToAClientGoEnum verifies the
// field type classifier over the shapes SDK fields take.
func TestEnumTypeOf_Types_UnwrapsPointersAndSlicesToAClientGoEnum(t *testing.T) {
	sdkPkg := types.NewPackage(shared.ClientGoPkgPath+"/v2", "gitlab")
	color := types.NewNamed(types.NewTypeName(0, sdkPkg, "ColorValue", nil), types.Typ[types.String], nil)
	plain := types.NewNamed(types.NewTypeName(0, sdkPkg, "PlainID", nil), types.Typ[types.String], nil)
	local := types.NewNamed(types.NewTypeName(0, types.NewPackage("example.com/x", "x"), "ColorValue", nil), types.Typ[types.String], nil)
	enums := map[string]sdkEnum{"ColorValue": {Name: "ColorValue"}}
	cases := []struct {
		name string
		typ  types.Type
		want string
	}{
		{name: "named", typ: color, want: "ColorValue"},
		{name: "pointer", typ: types.NewPointer(color), want: "ColorValue"},
		{name: "slice", typ: types.NewSlice(color), want: "ColorValue"},
		{name: "pointer_to_slice_of_pointers", typ: types.NewPointer(types.NewSlice(types.NewPointer(color))), want: "ColorValue"},
		{name: "named_without_constants", typ: plain, want: ""},
		{name: "same_name_outside_client_go", typ: local, want: ""},
		{name: "basic", typ: types.Typ[types.String], want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := enumTypeOf(tc.typ, enums); got != tc.want {
				t.Errorf("enumTypeOf = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCollectTaggedFields_Structs_FlattenEmbedsAndStopAtTheDepthLimit
// verifies the field walker: embedded structs are flattened through a
// pointer, an excluded or untagged field is skipped, and the recursion stops
// at the depth limit rather than descending forever.
func TestCollectTaggedFields_Structs_FlattenEmbedsAndStopAtTheDepthLimit(t *testing.T) {
	field := func(name, tag string) (*types.Var, string) {
		return types.NewField(0, nil, name, types.Typ[types.String], false), tag
	}
	leafVar, leafTag := field("Leaf", `json:"leaf"`)
	skipVar, skipTag := field("Skip", `json:"-"`)
	bareVar, bareTag := field("Bare", ``)
	leaf := types.NewStruct([]*types.Var{leafVar, skipVar, bareVar}, []string{leafTag, skipTag, bareTag})
	leafNamed := types.NewNamed(types.NewTypeName(0, nil, "Leaf", nil), leaf, nil)
	embed := types.NewStruct([]*types.Var{types.NewField(0, nil, "Leaf", types.NewPointer(leafNamed), true)}, []string{""})

	got := taggedFields(embed, []string{tagJSON})
	if len(got) != 1 || got[0].tag != "leaf" || got[0].name != "Leaf" {
		t.Errorf("taggedFields = %+v, want the one embedded leaf field", got)
	}
	if got := tagsOf(embed, []string{tagJSON}); !reflect.DeepEqual(got, map[string]struct{}{"leaf": {}}) {
		t.Errorf("tagsOf = %v, want {leaf}", got)
	}

	// A struct nested in itself through embedding cannot be built with
	// go/types, so the limit is exercised by starting below it.
	var out []taggedField
	collectTaggedFields(leaf, []string{tagJSON}, &out, maxEmbedDepth+1)
	if len(out) != 0 {
		t.Errorf("collectTaggedFields past the depth limit = %+v, want nothing", out)
	}
	collectTaggedFields(nil, []string{tagJSON}, &out, 0)
	if len(out) != 0 {
		t.Errorf("collectTaggedFields(nil) = %+v, want nothing", out)
	}
	if _, ok := structUnder(types.Typ[types.Int]); ok {
		t.Error("structUnder(int) reported a struct")
	}
}

// TestHelpers_Small_BehaveAsDocumented covers the small pure helpers: set
// difference is sorted and nil when empty, the finding order is action then
// kind then field, matchTag falls back to the normalized form, and an SDK
// enum's underlying kind is classified.
func TestHelpers_Small_BehaveAsDocumented(t *testing.T) {
	t.Run("difference", func(t *testing.T) {
		if got := difference([]string{"b", "a", "c"}, []string{"c"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("difference = %v, want [a b]", got)
		}
		if got := difference([]string{"a"}, []string{"a"}); got != nil {
			t.Errorf("difference of equal sets = %v, want nil", got)
		}
	})
	t.Run("findingLess", func(t *testing.T) {
		cases := []struct {
			name string
			a, b Finding
			want bool
		}{
			{name: "by_action", a: Finding{Action: "a.x"}, b: Finding{Action: "b.x"}, want: true},
			{name: "by_kind", a: Finding{Action: "a", Kind: kindInput}, b: Finding{Action: "a", Kind: kindOutput}, want: true},
			{name: "by_field", a: Finding{Action: "a", Kind: kindInput, Field: "z"}, b: Finding{Action: "a", Kind: kindInput, Field: "y"}, want: false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := findingLess(tc.a, tc.b); got != tc.want {
					t.Errorf("findingLess = %v, want %v", got, tc.want)
				}
			})
		}
	})
	t.Run("matchTag", func(t *testing.T) {
		tags := map[string]struct{}{"iids": {}, "not_author_id": {}}
		cases := []struct {
			name   string
			tag    string
			want   string
			wantOK bool
		}{
			{name: "exact", tag: "iids", want: "iids", wantOK: true},
			{name: "array_suffix", tag: "iids[]", want: "iids", wantOK: true},
			{name: "negation", tag: "not[author_id]", want: "not_author_id", wantOK: true},
			{name: "absent", tag: "state", want: "", wantOK: false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, ok := matchTag(tc.tag, tags)
				if got != tc.want || ok != tc.wantOK {
					t.Errorf("matchTag(%q) = %q/%v, want %q/%v", tc.tag, got, ok, tc.want, tc.wantOK)
				}
			})
		}
	})
	t.Run("isEnumUnderlying", func(t *testing.T) {
		cases := []struct {
			name string
			typ  types.Type
			want bool
		}{
			{name: "string", typ: types.Typ[types.String], want: true},
			{name: "int64", typ: types.Typ[types.Int64], want: true},
			{name: "float", typ: types.Typ[types.Float64], want: false},
			{name: "struct", typ: types.NewStruct(nil, nil), want: false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				named := types.NewNamed(types.NewTypeName(0, nil, "T", nil), tc.typ, nil)
				if got := isEnumUnderlying(named); got != tc.want {
					t.Errorf("isEnumUnderlying = %v, want %v", got, tc.want)
				}
			})
		}
	})
}

// TestAnalyze_Roots_LoadTheTreeOrFail verifies the entry points against a
// module without client-go (a structural precondition, reported rather than
// audited as clean) and against the fixture, whose structs no catalog action
// routes, so the real catalog offers nothing and the report is clean with
// zero fields.
func TestAnalyze_Roots_LoadTheTreeOrFail(t *testing.T) {
	t.Run("no_client_go_in_the_import_graph", func(t *testing.T) {
		root := t.TempDir()
		writeFiles(t, root, map[string]string{
			"go.mod":                        "module example.com/plain\n\ngo 1.27\n",
			"internal/tools/alpha/alpha.go": "package alpha\n\nfunc A() int { return 1 }\n",
		})
		if _, err := Analyze(root, true); err == nil || !strings.Contains(err.Error(), "client-go root package not found") {
			t.Fatalf("Analyze = %v, want a missing client-go error", err)
		}
	})
	t.Run("missing_root", func(t *testing.T) {
		content, clean, err := Run(filepath.Join(t.TempDir(), "absent"), true)
		if err == nil || !strings.Contains(err.Error(), "load packages") {
			t.Fatalf("Run on a missing root = %v, want a load error", err)
		}
		if clean || content != nil {
			t.Errorf("failed run returned clean=%v content=%d bytes, want false and nil", clean, len(content))
		}
	})
	t.Run("fixture_against_the_real_catalog", func(t *testing.T) {
		rep, err := Analyze(fixtureModule(t, nil), false)
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		// No catalog action routes a fixture struct, so nothing is held to
		// anything, and every committed exemption excuses nothing here: the
		// staleness rule reports all of them, which is what keeps the table
		// honest on the real tree.
		if rep.Summary.Fields != 0 || rep.Summary.SDKEnums != 2 || rep.Summary.MissingValues != 0 || rep.Summary.ExtraValues != 0 {
			t.Errorf("fixture summary = %+v, want two SDK enums and no routed field", rep.Summary)
		}
		if rep.Summary.StaleExemptions != len(acceptedEnumGaps) || rep.Summary.Clean() {
			t.Errorf("fixture summary = %+v, want every one of the %d exemptions stale and a red gate", rep.Summary, len(acceptedEnumGaps))
		}
	})
}

// TestRun_Repository_GateIsGreenAndShapeIsStable runs the real repository
// through the command-facing entry point: the gate passes with the committed
// exemption table, every exemption is in use, the JSON carries the shared
// header and a trailing newline, and -gaps-only leaves the package list
// empty because there is nothing to list. The Dependency Firewall ecosystems
// are the case the rule was written for, so the full report is checked to
// hold that field to client-go's constants through a schema enum.
func TestRun_Repository_GateIsGreenAndShapeIsStable(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, clean, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !clean {
		t.Errorf("enum gate is red; run `make audit-1to1-enums`:\n%s", content)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("report lacks the trailing newline")
	}
	var rep Report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.SchemaVersion != shared.SchemaVersion || rep.ClientGoPath != shared.ClientGoPkgPath {
		t.Errorf("report header = %d/%q, want %d/%q", rep.SchemaVersion, rep.ClientGoPath, shared.SchemaVersion, shared.ClientGoPkgPath)
	}
	if len(rep.Packages) != 0 || len(rep.StaleExemptions) != 0 {
		t.Errorf("gaps-only report kept %d packages and %d stale exemptions, want none", len(rep.Packages), len(rep.StaleExemptions))
	}
	if rep.Summary.SDKEnums < 50 || rep.Summary.Fields < 500 {
		t.Errorf("summary = %+v, want the SDK's full enum surface and every routed field", rep.Summary)
	}

	full, err := Analyze(root, false)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	ecosystem, ok := findingsByField(full)["input ecosystem"]
	if !ok || ecosystem.Enum != "DependencyFirewallEcosystemValue" || ecosystem.Source != sourceEnum || ecosystem.hasGap() {
		t.Errorf("dependency firewall ecosystem finding = %+v, want an enum-sourced clean field held to DependencyFirewallEcosystemValue", ecosystem)
	}
	if len(ecosystem.SDKValues) < 11 || !reflect.DeepEqual(ecosystem.Offered, ecosystem.SDKValues) {
		t.Errorf("ecosystem offered %v, sdk %v; want the two equal with client-go's full set", ecosystem.Offered, ecosystem.SDKValues)
	}
}

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
	if _, _, err = Run(root, true); err == nil || !strings.Contains(err.Error(), "marshal report: boom") {
		t.Fatalf("Run = %v, want the marshal failure", err)
	}
}

// TestAcceptedEnumGaps_Entries_AreWellFormedAndCiteTheirEvidence verifies
// the exemption table itself: every key is a field or a field=value form
// under a package, every reason names the GitLab doc or the upstream
// register it rests on, and the generated feature access level family
// covers both project inputs.
func TestAcceptedEnumGaps_Entries_AreWellFormedAndCiteTheirEvidence(t *testing.T) {
	if len(acceptedEnumGaps) == 0 {
		t.Fatal("acceptedEnumGaps is empty")
	}
	for key, reason := range acceptedEnumGaps {
		t.Run(key, func(t *testing.T) {
			field, _, _ := strings.Cut(key, "=")
			if parts := strings.Split(field, "."); len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
				t.Errorf("key %q is not <pkg>.<MCPType>.<tag>[=<value>]", key)
			}
			if !strings.Contains(reason, "doc/api/") && !strings.Contains(reason, "upstream-bugs.md") {
				t.Errorf("reason for %q cites neither a doc/api page nor the upstream register: %q", key, reason)
			}
		})
	}
	generated := projectFeatureAccessLevelExemptions()
	if len(generated) != 2*len(projectFeatureAccessLevels) {
		t.Errorf("generated %d feature exemptions, want %d", len(generated), 2*len(projectFeatureAccessLevels))
	}
	for key := range generated {
		if _, ok := acceptedEnumGaps[key]; !ok {
			t.Errorf("generated key %q is not in the table", key)
		}
	}
}
