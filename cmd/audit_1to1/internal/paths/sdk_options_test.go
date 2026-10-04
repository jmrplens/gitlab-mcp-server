package paths

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestReadSDKOptions_TheStructAMethodIsGiven_IsReadWithItsRoutes verifies the
// first link of the always-sent join: which option struct reaches which
// endpoint, and which of its keys encoding/json writes whatever the caller
// filled in.
//
// Three of the cases are the ones that would silently unmake the rule. A field
// tagged omitzero is omitted exactly as one tagged omitempty is, and reading
// only omitempty reported four of client-go's slices as values a caller cannot
// decline to send. An embedded option struct's fields are promoted, so they are
// the enclosing struct's own as far as a body is concerned. And every method
// ends in the variadic transport tail, RequestOptionFunc, so a rule that read
// every parameter would attribute every route to it.
func TestReadSDKOptions_TheStructAMethodIsGiven_IsReadWithItsRoutes(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var (
	routeProjectsIDRules   = route("projects/%s/packages/protection/rules")
	routeProjectsIDRulesID = route("projects/%s/packages/protection/rules/%d")
)
`,
		"options.go": `package gitlab

type UpdateRulesOptions struct {
	ListOptions
	PackageNamePattern *string             ` + "`" + `url:"package_name_pattern" json:"package_name_pattern"` + "`" + `
	MinimumAccessLevel Nullable[Access]    ` + "`" + `json:"minimum_access_level,omitempty"` + "`" + `
	AllowedToPush      []*GroupAccessLevel ` + "`" + `json:"allowed_to_push,omitzero"` + "`" + `
	Position           *PositionOptions    ` + "`" + `json:"position,omitempty"` + "`" + `
	Untagged           *string
	Hidden             *string             ` + "`" + `json:"-"` + "`" + `
	unexported         *string             ` + "`" + `json:"unexported"` + "`" + `
}

type PositionOptions struct {
	PositionType *string ` + "`" + `json:"position_type"` + "`" + `
}

type ListOptions struct {
	Page *int ` + "`" + `json:"page,omitempty"` + "`" + `
}

type NotAStructOptions = UpdateRulesOptions
`,
		"service.go": `package gitlab

import "net/http"

func (s *ProtectedPackagesService) UpdateRules(pid any, id int64, opt *UpdateRulesOptions, options ...RequestOptionFunc) (*Rule, *Response, error) {
	return do[*Rule](s.client,
		withMethod(http.MethodPatch),
		withPath(routeProjectsIDRulesID, ProjectID{pid}, id),
		withAPIOpts(opt),
	)
}

func (s *ProtectedPackagesService) ListRules(pid any, options ...RequestOptionFunc) ([]*Rule, *Response, error) {
	return do[[]*Rule](s.client, withPath(routeProjectsIDRules, ProjectID{pid}))
}
`,
	})

	options := readSDKOptions(dir)

	rules, known := options.Types["UpdateRulesOptions"]
	if !known {
		t.Fatalf("read %v, want UpdateRulesOptions among them", mapKeys(options.Types))
	}
	if !reflect.DeepEqual(rules.Embedded, []string{"ListOptions"}) {
		t.Errorf("embedded = %v, want [ListOptions]", rules.Embedded)
	}
	want := []sdkOptionField{
		{GoName: "PackageNamePattern", Name: "package_name_pattern", Always: true},
		{GoName: "MinimumAccessLevel", Name: "minimum_access_level"},
		{GoName: "AllowedToPush", Name: "allowed_to_push", Many: true},
		{GoName: "Position", Name: "position", Nested: "PositionOptions"},
		{GoName: "Untagged", Name: "Untagged", Always: true},
	}
	if !reflect.DeepEqual(rules.Fields, want) {
		t.Errorf("fields = %+v, want %+v", rules.Fields, want)
	}
	if routes := options.Routes["UpdateRulesOptions"]; len(routes) != 1 || routes[0].operation() != "PATCH /projects/:/packages/protection/rules/:" {
		t.Errorf("routes = %+v, want the one PATCH the method sends", routes)
	}
	if routes, claimed := options.Routes["RequestOptionFunc"]; claimed {
		t.Errorf("the transport's variadic tail claimed %+v, want no route at all", routes)
	}
}

// TestWalkOptionParams_NestedStructs_AreNamedTheWayGrapeDeclaresThem verifies
// the naming the comparison joins on. GitLab declares a nested param with its
// parent's key in front of it, so PositionOptions.PositionType is
// position[position_type] and nothing under a plain name would ever match.
//
// The self-reference is the case that decides whether this terminates at all,
// and a bound that only counted depth would still walk six pointless levels
// first.
func TestWalkOptionParams_NestedStructs_AreNamedTheWayGrapeDeclaresThem(t *testing.T) {
	types := map[string]sdkOptionType{
		"CreateOptions": {
			Embedded: []string{"SharedOptions"},
			Fields: []sdkOptionField{
				{GoName: "Note", Name: "note", Always: true},
				{GoName: "Position", Name: "position", Nested: "PositionOptions"},
				{GoName: "Actions", Name: "actions", Nested: "ActionOptions", Many: true},
			},
		},
		"SharedOptions":   {Fields: []sdkOptionField{{GoName: "Sudo", Name: "sudo"}}},
		"PositionOptions": {Fields: []sdkOptionField{{GoName: "PositionType", Name: "position_type", Always: true}}},
		"ActionOptions": {Fields: []sdkOptionField{
			{GoName: "Action", Name: "action", Always: true},
			{GoName: "Self", Name: "self", Nested: "CreateOptions"},
		}},
	}

	var params []string
	walkOptionParams(types, "CreateOptions", "", nil, 0, func(found optionParam) {
		params = append(params, found.Param)
	})

	want := []string{"sudo", "note", "position", "position[position_type]", "actions", "actions[][action]", "actions[][self]"}
	if !reflect.DeepEqual(params, want) {
		t.Errorf("params = %v, want %v", params, want)
	}
}

// TestWalkOptionParams_AChainDeeperThanTheBound_NamesTheDeepestParam verifies
// what the depth bound leaves the caller, with no type repeated for the
// ancestor check to catch: a chain of nested option structs longer than
// client-go's deepest is walked to the bound, the last param named with one
// bracket per level, and an embed chain promotes its fields from the deepest
// level the bound reaches.
func TestWalkOptionParams_AChainDeeperThanTheBound_NamesTheDeepestParam(t *testing.T) {
	types := map[string]sdkOptionType{}
	for level := range optionDepth + 3 {
		types[fmt.Sprintf("Level%dOptions", level)] = sdkOptionType{Fields: []sdkOptionField{
			{GoName: "Next", Name: "next", Nested: fmt.Sprintf("Level%dOptions", level+1)},
		}}
	}

	var params []string
	walkOptionParams(types, "Level0Options", "", nil, 0, func(found optionParam) {
		params = append(params, found.Param)
	})

	if len(params) != optionDepth+1 {
		t.Fatalf("visited %d params %v, want the %d levels the bound allows", len(params), params, optionDepth+1)
	}
	if want := "next" + strings.Repeat("[next]", optionDepth); params[optionDepth] != want {
		t.Errorf("deepest param = %q, want %q", params[optionDepth], want)
	}

	// An embed is a level too, though it adds nothing to the param's name.
	embeds := map[string]sdkOptionType{}
	for level := range optionDepth + 3 {
		embeds[fmt.Sprintf("Embed%dOptions", level)] = sdkOptionType{
			Embedded: []string{fmt.Sprintf("Embed%dOptions", level+1)},
			Fields:   []sdkOptionField{{GoName: "Own", Name: fmt.Sprintf("own%d", level)}},
		}
	}
	var promoted []string
	walkOptionParams(embeds, "Embed0Options", "", nil, 0, func(found optionParam) {
		promoted = append(promoted, found.Param)
	})
	if len(promoted) != optionDepth+1 || promoted[0] != fmt.Sprintf("own%d", optionDepth) {
		t.Errorf("promoted = %v, want own%d down to own0, the %d levels the bound allows", promoted, optionDepth, optionDepth+1)
	}
}

// TestSDKOptionReaders_EmbedsAndTags verifies two readings the client-go
// source exercises and the fixture above does not: an embedded type that is
// not an option struct contributes nothing, and a json key spelled "-," is the
// key "-" rather than a hidden field. Which option structs a method takes, each
// once and the variadic tail never, is cmd/internal/sdkroutes' reading and is
// held by its own suite.
func TestSDKOptionReaders_EmbedsAndTags(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package gitlab

type WithBaseOptions struct {
	Base
	ListOptions
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	structType := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	if got := optionFields(structType); !reflect.DeepEqual(got.Embedded, []string{"ListOptions"}) {
		t.Errorf("embedded = %v, want only the option struct", got.Embedded)
	}
	key, always, published := jsonField(&ast.BasicLit{Kind: token.STRING, Value: "`json:\"-,\"`"}, "Dash")
	if key != "-" || !always || !published {
		t.Errorf("jsonField(-,) = %q, %t, %t; want the key \"-\", always, published", key, always, published)
	}
}

// TestWalkOptionParams_AnUnknownType_VisitsNothing verifies that a field naming
// a struct the parse never read contributes no param, since inventing one would
// produce a comparison against a name GitLab was never asked about.
func TestWalkOptionParams_AnUnknownType_VisitsNothing(t *testing.T) {
	visited := 0
	walkOptionParams(map[string]sdkOptionType{}, "MissingOptions", "", nil, 0, func(optionParam) { visited++ })

	if visited != 0 {
		t.Errorf("visited %d param(s), want none", visited)
	}
}

// TestReadSDKOptions_AnUnreadableDirectory_ReadsNothing verifies the silence
// this shares with every other reader of the module cache: a directory it
// cannot parse leaves the check with nothing to say rather than failing the
// scope over somebody else's source tree.
func TestReadSDKOptions_AnUnreadableDirectory_ReadsNothing(t *testing.T) {
	for _, dir := range []string{"", t.TempDir(), filepath.Join(t.TempDir(), "absent")} {
		t.Run(dir, func(t *testing.T) {
			if options := readSDKOptions(dir); len(options.Types) != 0 || len(options.Routes) != 0 {
				t.Errorf("readSDKOptions(%q) = %+v, want nothing", dir, options)
			}
		})
	}
}

// TestSDKOptionReaders_ShapesClientGoHasNotUsed verifies the readers' answers
// for the shapes client-go's source does not use today and a later release
// could: a struct with no field list reads as no field, and a tag that is not
// a string literal and a json tag that names no key both leave the field under
// its Go name as encoding/json would, the second omitted when empty as its
// option says.
func TestSDKOptionReaders_ShapesClientGoHasNotUsed(t *testing.T) {
	if got := optionFields(&ast.StructType{}); len(got.Fields) != 0 || len(got.Embedded) != 0 {
		t.Errorf("optionFields(no field list) = %+v, want nothing", got)
	}
	for _, tt := range []struct {
		name       string
		tag        *ast.BasicLit
		wantAlways bool
	}{
		{name: "a tag that is not a string", tag: &ast.BasicLit{Kind: token.INT, Value: "1"}, wantAlways: true},
		{name: "a json tag naming no key", tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\",omitempty\"`"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key, always, published := jsonField(tt.tag, "GoName")
			if key != "GoName" || always != tt.wantAlways || !published {
				t.Errorf("jsonField() = %q, %t, %t; want GoName, %t, true", key, always, published, tt.wantAlways)
			}
		})
	}
}

// TestParseSDKFiles_WhatItReads_IsTheSourceAndNothingBeside verifies which
// files the option structs are read from: the Go source directly in the
// directory, and not a test file, a file that does not parse, a file that is
// not named as Go source although it parses as Go, or anything in a
// subdirectory, each of which would add a struct client-go does not send.
func TestParseSDKFiles_WhatItReads_IsTheSourceAndNothingBeside(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"options.go":      "package gitlab\n\ntype GoodOptions struct{}\n",
		"options_test.go": "package gitlab\n\ntype FromATestOptions struct{}\n",
		"broken.go":       "package gitlab\n\ntype Unclosed struct {\n",
		"notes.txt":       "package gitlab\n\ntype FromProseOptions struct{}\n",
	})
	if err := os.Mkdir(filepath.Join(dir, "nested.go"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	files := parseSDKFiles(dir)

	if len(files) != 1 || files[0].Name.Name != "gitlab" || len(files[0].Decls) != 1 {
		t.Fatalf("parseSDKFiles() read %d file(s), want options.go alone", len(files))
	}
	if options := readSDKOptions(dir); !reflect.DeepEqual(mapKeys(options.Types), []string{"GoodOptions"}) {
		t.Errorf("readSDKOptions() read %v, want [GoodOptions]", mapKeys(options.Types))
	}
}

// TestStringLiteral_ALiteralTheParserWouldRefuse_IsNotRead verifies the one
// branch a parsed fixture cannot reach: go/parser rejects a file holding a
// string it cannot unquote, so the guard is only reachable from a node built by
// hand. It is kept because this walks an AST it did not build the invariants
// of, and reading a broken tag as a key would name a param nothing sends.
func TestStringLiteral_ALiteralTheParserWouldRefuse_IsNotRead(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "not a literal at all", expr: ast.NewIdent("path")},
		{name: "a literal that is not a string", expr: &ast.BasicLit{Kind: token.INT, Value: "5"}},
		{name: "a string that does not unquote", expr: &ast.BasicLit{Kind: token.STRING, Value: `"unterminated`}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if value, ok := stringLiteral(testCase.expr); ok {
				t.Errorf("stringLiteral() = %q, true; want it refused", value)
			}
		})
	}
}

// TestJSONField_OnlyABareDashHidesTheField verifies the one spelling that
// hides a field from encoding/json and the two that look like it and do not:
// "-," and "-,omitempty" write the field under the key "-", the second only
// when it holds something.
func TestJSONField_OnlyABareDashHidesTheField(t *testing.T) {
	cases := []struct {
		name          string
		tag           string
		wantKey       string
		wantAlways    bool
		wantPublished bool
	}{
		{name: "a bare dash", tag: "`json:\"-\"`"},
		{name: "a dash and a comma", tag: "`json:\"-,\"`", wantKey: "-", wantAlways: true, wantPublished: true},
		{name: "a dash with omitempty", tag: "`json:\"-,omitempty\"`", wantKey: "-", wantPublished: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			key, always, published := jsonField(&ast.BasicLit{Kind: token.STRING, Value: testCase.tag}, "GoName")
			if key != testCase.wantKey || always != testCase.wantAlways || published != testCase.wantPublished {
				t.Errorf("jsonField(%s) = %q, %t, %t; want %q, %t, %t", testCase.tag, key, always, published,
					testCase.wantKey, testCase.wantAlways, testCase.wantPublished)
			}
		})
	}
}

// TestOptionFields_AnEmbedThatIsNoOptionStruct_IsNotPromoted verifies that an
// embedded struct is read as more fields of the option struct only when it is
// an option struct itself; any other embed is left alone, since the walk would
// find no fields for it and would name it in the output as though it had some.
func TestOptionFields_AnEmbedThatIsNoOptionStruct_IsNotPromoted(t *testing.T) {
	structType := parseStruct(t, "ListOptions\n\tPagination\n\tName *string `json:\"name,omitempty\"`")

	got := optionFields(structType)

	if !reflect.DeepEqual(got.Embedded, []string{"ListOptions"}) {
		t.Errorf("Embedded = %v, want the option struct alone", got.Embedded)
	}
}

// TestWalkOptionParams_AChainDeeperThanTheBound_StopsAtIt verifies the depth
// bound on its own, with no type naming itself: a chain of distinct option
// structs deeper than the bound is walked to the bound and no further, which is
// what keeps a pathological source from walking without end even where the
// ancestor check sees nothing repeat.
func TestWalkOptionParams_AChainDeeperThanTheBound_StopsAtIt(t *testing.T) {
	types := map[string]sdkOptionType{}
	for level := 0; level <= optionDepth+1; level++ {
		name := fmt.Sprintf("Level%dOptions", level)
		types[name] = sdkOptionType{Fields: []sdkOptionField{
			{GoName: "Next", Name: "next", Nested: fmt.Sprintf("Level%dOptions", level+1)},
		}}
	}

	visited := 0
	walkOptionParams(types, "Level0Options", "", nil, 0, func(optionParam) { visited++ })

	if visited != optionDepth+1 {
		t.Errorf("visited %d param(s), want %d, one per level up to the bound", visited, optionDepth+1)
	}
}

// TestWalkOptionParams_AnEmbedChainDeeperThanTheBound_StopsAtIt verifies that
// an embed spends the depth the way a nested field does. An embed promotes its
// fields rather than naming them under a key, so it is easy to treat as free,
// and a chain of distinct embeds, which the ancestor check never stops, would
// then be walked as deep as it goes.
func TestWalkOptionParams_AnEmbedChainDeeperThanTheBound_StopsAtIt(t *testing.T) {
	types := map[string]sdkOptionType{}
	for level := 0; level <= optionDepth+1; level++ {
		types[fmt.Sprintf("Level%dOptions", level)] = sdkOptionType{
			Embedded: []string{fmt.Sprintf("Level%dOptions", level+1)},
			Fields:   []sdkOptionField{{GoName: "Own", Name: fmt.Sprintf("own_%d", level)}},
		}
	}

	visited := 0
	walkOptionParams(types, "Level0Options", "", nil, 0, func(optionParam) { visited++ })

	if visited != optionDepth+1 {
		t.Errorf("visited %d param(s), want %d, one per level up to the bound", visited, optionDepth+1)
	}
}

// mapKeys names a map's keys for a failure message.
func mapKeys[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}
