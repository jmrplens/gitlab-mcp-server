// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkoptions

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// sourceIn writes a directory of pretend client-go source and returns it.
func sourceIn(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// tick spells a backquote, so a struct tag can sit inside a raw string.
const tick = "`"

// optionsSource is the stand-in the reading is held to: one option struct
// carrying each shape a field takes, the types defined on it, and two
// methods, one built with NewRequest and one with an upload.
var optionsSource = map[string]string{
	"routes.go": `package gitlab

var (
	routeProjectsIDRulesID = route("projects/%s/packages/protection/rules/%d")
	routeProjectsIDAvatar  = route("projects/%s/avatar")
)
`,
	"options.go": `package gitlab

import "time"

type UpdateRulesOptions struct {
	ListOptions
	Pagination
	time.Location
	PackageNamePattern *string              ` + tick + `url:"package_name_pattern" json:"package_name_pattern"` + tick + `
	MinimumAccessLevel Nullable[Access]     ` + tick + `json:"minimum_access_level,omitempty"` + tick + `
	AllowedToPush      []*GroupAccessLevel  ` + tick + `url:"allowed_to_push,omitempty" json:"allowed_to_push,omitzero"` + tick + `
	Position           *PositionOptions     ` + tick + `url:"position,omitempty" json:"position,omitempty"` + tick + `
	CustomHeaders      *[]*HookCustomHeader ` + tick + `url:"custom_headers,omitempty" json:"custom_headers,omitempty"` + tick + `
	Labels             []string             ` + tick + `url:"labels" json:"labels"` + tick + `
	Created            *ISOTime             ` + tick + `url:"created,omitempty" json:"created,omitempty"` + tick + `
	Access             *AccessValue         ` + tick + `url:"access" json:"access"` + tick + `
	Attributes         map[string]*Attr     ` + tick + `json:"attributes,omitempty"` + tick + `
	Untagged           *string
	Hidden             *string              ` + tick + `url:"hidden,omitempty" json:"-"` + tick + `
	unexported         *string              ` + tick + `json:"unexported"` + tick + `
}

type PositionOptions struct {
	PositionType *string ` + tick + `url:"position_type" json:"position_type"` + tick + `
}

type HookCustomHeader struct {
	Key   string ` + tick + `json:"key"` + tick + `
	Value string ` + tick + `json:"value"` + tick + `
}

type Attr struct {
	Value *string ` + tick + `json:"value,omitempty"` + tick + `
}

type GroupAccessLevel struct {
	AccessLevel *int ` + tick + `json:"access_level,omitempty"` + tick + `
}

type ListOptions struct {
	Page *int ` + tick + `url:"page,omitempty" json:"page,omitempty"` + tick + `
}

type ForUserOptions UpdateRulesOptions

type AliasOptions = UpdateRulesOptions

type DeeperOptions ForUserOptions

type ISOTime time.Time

type AccessValue string

type UploadType string

const UploadAvatar UploadType = "avatar"

type LoopAOptions LoopBOptions

type LoopBOptions LoopAOptions
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

func (s *ProjectsService) UploadAvatar(pid any, opt *ForUserOptions, options ...RequestOptionFunc) (*Rule, *Response, error) {
	return do[*Rule](s.client,
		withMethod(http.MethodPut),
		withPath(routeProjectsIDAvatar, ProjectID{pid}),
		withUpload(nil, "avatar", UploadAvatar),
		withAPIOpts(opt),
	)
}
`,
}

// TestRead_TheStructAMethodIsGiven_IsReadForBothEncoders verifies the first
// link of every option struct check: which struct reaches which endpoint
// through which constructor, and what each encoder writes of it.
//
// The cases are the ones that would silently unmake a check. A field tagged
// omitzero is omitted by encoding/json exactly as one tagged omitempty is,
// while go-querystring reads omitempty alone, and a bare slice it skips when
// empty whatever its tag says. An embedded struct's fields are promoted, so
// they are the enclosing struct's own as far as a request is concerned, and
// an embed that is no struct of the package promotes nothing. A field
// carrying a struct that is not an option struct (HookCustomHeader) is still
// walked into, through a pointer to a slice or a map, while a field of a
// type defined on a scalar (ISOTime, AccessValue) is the scalar it is. And a
// type defined on an option struct (CreateProjectForUserOptions is declared
// as CreateProjectOptions) is that struct, through an alias and a definition
// on a definition alike.
func TestRead_TheStructAMethodIsGiven_IsReadForBothEncoders(t *testing.T) {
	sdk := Read(sourceIn(t, optionsSource))

	rules, known := sdk.Types["UpdateRulesOptions"]
	if !known {
		t.Fatalf("read %v, want UpdateRulesOptions among them", slices.Sorted(maps.Keys(sdk.Types)))
	}
	if !reflect.DeepEqual(rules.Embedded, []string{"ListOptions"}) {
		t.Errorf("embedded = %v, want [ListOptions]", rules.Embedded)
	}
	tag := func(name string, always bool) Tag { return Tag{Name: name, Always: always} }
	want := []Field{
		{GoName: "PackageNamePattern", JSON: tag("package_name_pattern", true), Query: tag("package_name_pattern", true)},
		{GoName: "MinimumAccessLevel", JSON: tag("minimum_access_level", false), Query: tag("MinimumAccessLevel", true)},
		{GoName: "AllowedToPush", JSON: tag("allowed_to_push", false), Query: tag("allowed_to_push", false), Nested: "GroupAccessLevel", Many: true},
		{GoName: "Position", JSON: tag("position", false), Query: tag("position", false), Nested: "PositionOptions"},
		{GoName: "CustomHeaders", JSON: tag("custom_headers", false), Query: tag("custom_headers", false), Nested: "HookCustomHeader", Many: true},
		{GoName: "Labels", JSON: tag("labels", true), Query: tag("labels", false), Many: true},
		{GoName: "Created", JSON: tag("created", false), Query: tag("created", false)},
		{GoName: "Access", JSON: tag("access", true), Query: tag("access", true)},
		{GoName: "Attributes", JSON: tag("attributes", false), Query: tag("Attributes", true), Nested: "Attr"},
		{GoName: "Untagged", JSON: tag("Untagged", true), Query: tag("Untagged", true)},
		{GoName: "Hidden", JSON: Tag{Hidden: true}, Query: tag("hidden", false)},
	}
	if !reflect.DeepEqual(rules.Fields, want) {
		t.Errorf("fields =\n%+v\nwant\n%+v", rules.Fields, want)
	}
	for _, name := range []string{"ForUserOptions", "AliasOptions", "DeeperOptions"} {
		t.Run(name, func(t *testing.T) {
			if got := sdk.Types[name]; !reflect.DeepEqual(got, rules) {
				t.Errorf("%s = %+v, want the struct it is defined on", name, got)
			}
		})
	}
	for _, name := range []string{"ISOTime", "AccessValue", "UploadType", "LoopAOptions", "LoopBOptions"} {
		t.Run(name, func(t *testing.T) {
			if got, read := sdk.Types[name]; read {
				t.Errorf("%s = %+v, want no struct for a type defined on none", name, got)
			}
		})
	}

	update := sdk.Methods["ProtectedPackages.UpdateRules"]
	if !reflect.DeepEqual(update.Options, []string{"UpdateRulesOptions"}) {
		t.Errorf("options = %v, want the one option struct the method takes and not the variadic tail", update.Options)
	}
	if len(update.Routes) != 1 || update.Routes[0].String() != "PATCH /projects/:/packages/protection/rules/:" {
		t.Errorf("routes = %+v, want the one PATCH the method sends", update.Routes)
	}
	if !reflect.DeepEqual(update.Constructors, []string{sdkroutes.ConstructorNewRequest}) {
		t.Errorf("constructors = %v, want NewRequest", update.Constructors)
	}
	avatar := sdk.Methods["Projects.UploadAvatar"]
	if !reflect.DeepEqual(avatar.Constructors, []string{sdkroutes.ConstructorUpload}) || !reflect.DeepEqual(avatar.UploadFields, []string{"avatar"}) {
		t.Errorf("upload = %+v, want UploadRequest sending its file as avatar", avatar)
	}
}

// TestEncoders_TheConstructorAndTheVerb_DecideTheTagRead verifies the
// encoder each request writes its options with: NewRequest's own switch,
// which sends the options of a GET, a DELETE and a HEAD as a query read off
// the url tag, and an upload, which writes them as multipart fields through
// the url tag whatever the verb. A method that can build either is judged by
// both.
func TestEncoders_TheConstructorAndTheVerb_DecideTheTagRead(t *testing.T) {
	newRequest, upload := sdkroutes.ConstructorNewRequest, sdkroutes.ConstructorUpload
	cases := []struct {
		name         string
		constructors []string
		verb         string
		want         []string
	}{
		{name: "a POST body", constructors: []string{newRequest}, verb: "POST", want: []string{EncoderJSON}},
		{name: "a PUT body", constructors: []string{newRequest}, verb: "PUT", want: []string{EncoderJSON}},
		{name: "a PATCH body spelled in lowercase", constructors: []string{newRequest}, verb: "patch", want: []string{EncoderJSON}},
		{name: "a GET query", constructors: []string{newRequest}, verb: "GET", want: []string{EncoderQuery}},
		{name: "a DELETE query", constructors: []string{newRequest}, verb: "DELETE", want: []string{EncoderQuery}},
		{name: "a HEAD query", constructors: []string{newRequest}, verb: "HEAD", want: []string{EncoderQuery}},
		{name: "an upload under POST", constructors: []string{upload}, verb: "POST", want: []string{EncoderQuery}},
		{name: "either under PUT", constructors: []string{upload, newRequest}, verb: "PUT", want: []string{EncoderJSON, EncoderQuery}},
		{name: "either under GET, each once", constructors: []string{newRequest, upload}, verb: "GET", want: []string{EncoderQuery}},
		{name: "a constructor neither names", constructors: []string{"Other"}, verb: "POST"},
		{name: "no constructor", verb: "POST"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Encoders(tt.constructors, tt.verb); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Encoders(%v, %s) = %v, want %v", tt.constructors, tt.verb, got, tt.want)
			}
		})
	}
}

// walkTypes is a hand-built set of structs each walk below reads: a list of
// structs, a nested struct, a field one encoder hides, an embed, and a
// struct that names itself.
func walkTypes() map[string]Type {
	both := func(name string, always bool) (Tag, Tag) {
		return Tag{Name: name, Always: always}, Tag{Name: name, Always: always}
	}
	field := func(goName, name string, always bool) Field {
		jsonTag, queryTag := both(name, always)
		return Field{GoName: goName, JSON: jsonTag, Query: queryTag}
	}
	nested := func(goName, name, inner string, many bool) Field {
		f := field(goName, name, false)
		f.Nested, f.Many = inner, many
		return f
	}
	bodyOnly := field("BodyOnly", "body_only", false)
	bodyOnly.Query = Tag{Hidden: true}
	return map[string]Type{
		"CreateOptions": {
			Embedded: []string{"SharedOptions"},
			Fields: []Field{
				field("Note", "note", true),
				nested("Position", "position", "PositionOptions", false),
				nested("Actions", "actions", "ActionOptions", true),
				bodyOnly,
			},
		},
		"SharedOptions":   {Fields: []Field{field("Sudo", "sudo", false)}},
		"PositionOptions": {Fields: []Field{field("PositionType", "position_type", true)}},
		"ActionOptions": {Fields: []Field{
			field("Action", "action", true),
			nested("Self", "self", "CreateOptions", false),
		}},
	}
}

// TestWalk_NestedStructs_AreNamedTheWayGrapeDeclaresThem verifies the naming
// every comparison joins on, under each encoder. GitLab declares a nested
// param with its parent's key in front of it, so
// PositionOptions.PositionType is position[position_type] and nothing under
// a plain name would ever match; encoding/json writes a list of structs as
// objects GitLab declares under an empty subscript, and go-querystring
// writes each element whole, so the list is walked into under the first
// alone. A field an encoder hides is left out under that encoder only.
//
// The self-reference is the case that decides whether this terminates at
// all, and a bound that only counted depth would still walk six pointless
// levels first.
func TestWalk_NestedStructs_AreNamedTheWayGrapeDeclaresThem(t *testing.T) {
	cases := []struct {
		encoder string
		want    []string
	}{
		{encoder: EncoderJSON, want: []string{"sudo", "note", "position", "position[position_type]", "actions", "actions[][action]", "actions[][self]", "body_only"}},
		{encoder: EncoderQuery, want: []string{"sudo", "note", "position", "position[position_type]", "actions"}},
	}
	for _, tt := range cases {
		t.Run(tt.encoder, func(t *testing.T) {
			var params []string
			Walk(walkTypes(), "CreateOptions", tt.encoder, func(found Param) {
				params = append(params, found.Param)
				if found.Tag != found.Field.Tag(tt.encoder) {
					t.Errorf("%s: tag = %+v, want the field's %s tag", found.Param, found.Tag, tt.encoder)
				}
			})
			if !reflect.DeepEqual(params, tt.want) {
				t.Errorf("params = %v, want %v", params, tt.want)
			}
		})
	}
}

// TestWalk_AChainDeeperThanTheBound_NamesTheDeepestParam verifies what the
// depth bound leaves the caller, with no type repeated for the ancestor
// check to catch: a chain of nested structs longer than client-go's deepest
// is walked to the bound, the last param named with one bracket per level,
// and an embed chain promotes its fields from the deepest level the bound
// reaches.
func TestWalk_AChainDeeperThanTheBound_NamesTheDeepestParam(t *testing.T) {
	types := map[string]Type{}
	for level := range optionDepth + 3 {
		types[fmt.Sprintf("Level%dOptions", level)] = Type{Fields: []Field{{
			GoName: "Next", JSON: Tag{Name: "next"}, Nested: fmt.Sprintf("Level%dOptions", level+1),
		}}}
	}

	var params []string
	Walk(types, "Level0Options", EncoderJSON, func(found Param) { params = append(params, found.Param) })

	if len(params) != optionDepth+1 {
		t.Fatalf("visited %d params %v, want the %d levels the bound allows", len(params), params, optionDepth+1)
	}
	if want := "next" + strings.Repeat("[next]", optionDepth); params[optionDepth] != want {
		t.Errorf("deepest param = %q, want %q", params[optionDepth], want)
	}

	// An embed is a level too, though it adds nothing to the param's name.
	embeds := map[string]Type{}
	for level := range optionDepth + 3 {
		embeds[fmt.Sprintf("Embed%dOptions", level)] = Type{
			Embedded: []string{fmt.Sprintf("Embed%dOptions", level+1)},
			Fields:   []Field{{GoName: "Own", JSON: Tag{Name: fmt.Sprintf("own%d", level)}}},
		}
	}
	var promoted []string
	Walk(embeds, "Embed0Options", EncoderJSON, func(found Param) { promoted = append(promoted, found.Param) })
	if len(promoted) != optionDepth+1 || promoted[0] != fmt.Sprintf("own%d", optionDepth) {
		t.Errorf("promoted = %v, want own%d down to own0, the %d levels the bound allows", promoted, optionDepth, optionDepth+1)
	}
}

// TestWalk_AnUnknownType_VisitsNothing verifies that a struct the parse never
// read contributes no param, since inventing one would produce a comparison
// against a name GitLab was never asked about.
func TestWalk_AnUnknownType_VisitsNothing(t *testing.T) {
	visited := 0
	Walk(map[string]Type{}, "MissingOptions", EncoderJSON, func(Param) { visited++ })

	if visited != 0 {
		t.Errorf("visited %d param(s), want none", visited)
	}
}

// TestRead_AnUnreadableDirectory_ReadsNothing verifies the silence this
// shares with every other reader of the module cache: a directory it cannot
// parse leaves the caller with nothing to say rather than failing it over
// somebody else's source tree.
func TestRead_AnUnreadableDirectory_ReadsNothing(t *testing.T) {
	for _, dir := range []string{"", t.TempDir(), filepath.Join(t.TempDir(), "absent")} {
		t.Run(dir, func(t *testing.T) {
			if sdk := Read(dir); len(sdk.Types) != 0 || len(sdk.Methods) != 0 {
				t.Errorf("Read(%q) = %+v, want nothing", dir, sdk)
			}
		})
	}
}

// TestStructFields_ShapesClientGoHasNotUsed verifies the reading of the
// shapes client-go's source does not use today and a later release could: a
// struct with no field list reads as no field, and an embed of another
// package's type promotes nothing.
func TestStructFields_ShapesClientGoHasNotUsed(t *testing.T) {
	if got := structFields(&ast.StructType{}); len(got.Fields) != 0 || len(got.Embedded) != 0 {
		t.Errorf("structFields(no field list) = %+v, want nothing", got)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package gitlab\n\ntype S struct {\n\tbytes.Buffer\n}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	structType := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	if got := structFields(structType); len(got.Embedded) != 0 {
		t.Errorf("embedded = %v, want nothing for another package's type", got.Embedded)
	}
}

// TestTagFor_EachSpelling_IsReadAsItsEncoderReadsIt verifies the tag rules
// both encoders share and the one they do not: a field with no tag, or no
// tag for the encoder, or a tag that is not a string literal, is written
// under its Go name and always; only a bare "-" hides the field, while "-,"
// and "-,omitempty" write it under the key "-"; a tag naming no key keeps the
// Go name; and omitzero omits under encoding/json alone, since
// go-querystring v1.2.0 does not read it.
func TestTagFor_EachSpelling_IsReadAsItsEncoderReadsIt(t *testing.T) {
	literal := func(tag string) *ast.BasicLit { return &ast.BasicLit{Kind: token.STRING, Value: tag} }
	cases := []struct {
		name    string
		tag     *ast.BasicLit
		encoder string
		want    Tag
	}{
		{name: "no tag at all", encoder: EncoderJSON, want: Tag{Name: "GoName", Always: true}},
		{name: "a tag that is not a string", tag: &ast.BasicLit{Kind: token.INT, Value: "1"}, encoder: EncoderJSON, want: Tag{Name: "GoName", Always: true}},
		{name: "no tag for the encoder", tag: literal("`json:\"name\"`"), encoder: EncoderQuery, want: Tag{Name: "GoName", Always: true}},
		{name: "a bare dash", tag: literal("`json:\"-\"`"), encoder: EncoderJSON, want: Tag{Hidden: true}},
		{name: "a dash and a comma", tag: literal("`url:\"-,\"`"), encoder: EncoderQuery, want: Tag{Name: "-", Always: true}},
		{name: "a dash with omitempty", tag: literal("`json:\"-,omitempty\"`"), encoder: EncoderJSON, want: Tag{Name: "-"}},
		{name: "a tag naming no key", tag: literal("`json:\",omitempty\"`"), encoder: EncoderJSON, want: Tag{Name: "GoName"}},
		{name: "omitzero under encoding/json", tag: literal("`json:\"name,omitzero\"`"), encoder: EncoderJSON, want: Tag{Name: "name"}},
		{name: "omitzero under go-querystring", tag: literal("`url:\"name,omitzero\"`"), encoder: EncoderQuery, want: Tag{Name: "name", Always: true}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagFor(tt.tag, tt.encoder, "GoName"); got != tt.want {
				t.Errorf("tagFor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseSDKFiles_WhatItReads_IsTheSourceAndNothingBeside verifies which
// files the structs are read from: the Go source directly in the directory,
// and not a test file, a file that does not parse, a file that is not named
// as Go source although it parses as Go, or anything in a subdirectory, each
// of which would add a struct client-go does not send.
func TestParseSDKFiles_WhatItReads_IsTheSourceAndNothingBeside(t *testing.T) {
	dir := sourceIn(t, map[string]string{
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
	if sdk := Read(dir); !reflect.DeepEqual(slices.Sorted(maps.Keys(sdk.Types)), []string{"GoodOptions"}) {
		t.Errorf("Read() read %v, want [GoodOptions]", slices.Sorted(maps.Keys(sdk.Types)))
	}
}

// TestStringLiteral_ALiteralTheParserWouldRefuse_IsNotRead verifies the one
// branch a parsed fixture cannot reach: go/parser rejects a file holding a
// string it cannot unquote, so the guard is only reachable from a node built
// by hand. It is kept because this walks an AST it did not build the
// invariants of, and reading a broken tag as a key would name a param
// nothing sends.
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

// TestCanonical_EverySpellingOfAList_IsTheRecordsName verifies the one
// rewrite every comparison with GitLab's record makes: a list's empty
// subscript, at the end of a key or between a parent and a nested key, is
// how an encoder and the request inventory spell a param the record names
// without it.
func TestCanonical_EverySpellingOfAList_IsTheRecordsName(t *testing.T) {
	cases := map[string]string{
		"iids[]":                 "iids",
		"custom_headers[][key]":  "custom_headers[key]",
		"not[author_id]":         "not[author_id]",
		"a[][b][][c]":            "a[b][c]",
		"plain":                  "plain",
		"approved_by_ids[]":      "approved_by_ids",
		"file.etag":              "file.etag",
		"actions[][file_path][]": "actions[file_path]",
	}
	for param, want := range cases {
		t.Run(param, func(t *testing.T) {
			if got := Canonical(param); got != want {
				t.Errorf("Canonical(%q) = %q, want %q", param, got, want)
			}
		})
	}
}
