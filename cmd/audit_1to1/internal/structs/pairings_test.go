// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// TestCollectOutputPairings_Repository_NamesTheSDKStructBehindEachOutput
// verifies the first link of the type-grain shape join. The paths scope asks
// GitLab's document what the endpoints one output type models actually send,
// and the only thing that says which endpoints those are is the client-go
// struct a converter fills the type from. A pairing lost here reads downstream
// as a type nothing routes to, which is silence rather than a finding.
func TestCollectOutputPairings_Repository_NamesTheSDKStructBehindEachOutput(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}

	pairings, err := CollectOutputPairings(root)
	if err != nil {
		t.Fatalf("collect the pairings: %v", err)
	}

	if pairings.ClientGoDir == "" {
		t.Fatal("no client-go directory found in the import graph the handlers compile against")
	}
	// What says the directory reads is the list, not the error: filepath.Glob
	// returns one only for a malformed pattern, and "*.go" is not one, so an
	// assertion on the error was true of a path that does not exist.
	sources, globErr := filepath.Glob(filepath.Join(pairings.ClientGoDir, "*.go"))
	if globErr != nil {
		t.Errorf("glob the client-go directory %q: %v", pairings.ClientGoDir, globErr)
	}
	if len(sources) == 0 {
		t.Errorf("client-go directory %q holds no Go source, so no service method could be read from it", pairings.ClientGoDir)
	}
	if len(pairings.Outputs) < 100 {
		t.Fatalf("collected %d pairing(s), want the converters across the whole tree", len(pairings.Outputs))
	}

	found := false
	for _, pairing := range pairings.Outputs {
		if strings.Contains(pairing.SDKType, ".") {
			t.Errorf("%s.%s names the SDK struct as %q, want it unqualified", pairing.Package, pairing.MCPType, pairing.SDKType)
		}
		if pairing.Package == "mrapprovals" && pairing.MCPType == "ConfigOutput" {
			found = true
			if pairing.SDKType != "MergeRequestApprovals" {
				t.Errorf("ConfigOutput pairs with %q, want MergeRequestApprovals", pairing.SDKType)
			}
			// The field list is the second half of the pairing and the only
			// input to the question of whether a field GitLab sends is absent
			// from client-go too. An empty or unsorted one would answer that
			// question wrongly without failing anything else here.
			if !slices.IsSorted(pairing.SDKFields) {
				t.Errorf("ConfigOutput's SDK fields are unsorted: %v", pairing.SDKFields)
			}
			if !slices.Contains(pairing.SDKFields, "approvals_required") {
				t.Errorf("ConfigOutput's SDK fields = %v, want the approvals_required key gl.MergeRequestApprovals carries", pairing.SDKFields)
			}
		}
	}
	if !found {
		t.Error("no pairing for mrapprovals.ConfigOutput, the one confirmed phantom this join was built for")
	}
}

// TestCollectOutputPairings_Repository_ReadsTheStructsAMergeRequestNests
// verifies the nested half of a pairing on the pairing it was built for: a
// merge request's head_pipeline is a gl.Pipeline, read to its own keys, which
// in client-go v3.30.0 carry name and no archived. The bump to a client-go
// that models archived is what this witness is meant to notice.
func TestCollectOutputPairings_Repository_ReadsTheStructsAMergeRequestNests(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}

	pairings, err := CollectOutputPairings(root)
	if err != nil {
		t.Fatalf("collect the pairings: %v", err)
	}

	index := slices.IndexFunc(pairings.Outputs, func(pairing OutputPairing) bool {
		return pairing.Package == "mergerequests" && pairing.MCPType == "Output" && pairing.SDKType == "MergeRequest"
	})
	if index < 0 {
		t.Fatal("no pairing of mergerequests.Output with gl.MergeRequest")
	}
	headPipeline := pairings.Outputs[index].SDKNested["head_pipeline"]
	if !slices.Contains(headPipeline, "name") || slices.Contains(headPipeline, "archived") {
		t.Errorf("the keys gl.MergeRequest nests under head_pipeline = %v, want gl.Pipeline's in v3.30.0: name and no archived", headPipeline)
	}
}

// TestCollectOutputPairings_ATreeThatDoesNotLoad_IsReported verifies that a
// load failure comes back as an error rather than as an empty pass. The scope
// above skips its whole type-grain join on one, and a nil error with no
// pairings would tell it there was nothing to compare.
func TestCollectOutputPairings_ATreeThatDoesNotLoad_IsReported(t *testing.T) {
	pairings, err := CollectOutputPairings(filepath.Join(t.TempDir(), "absent"))

	if err == nil {
		t.Fatalf("CollectOutputPairings() = %+v, nil; want the load failure reported", pairings)
	}
}

// TestDiffInputPairs_AStandInTree_ListsEveryPairWithTheFieldsItsInputLacks
// verifies the export the option structs section of the sdk scope reads:
// every input pair the field diff runs over, phantoms dropped, package by
// package and within one in the diff's order, each with both sides, how it
// was found and the SDK tags its input lacks that no declaration answers.
func TestDiffInputPairs_AStandInTree_ListsEveryPairWithTheFieldsItsInputLacks(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK,
		toolSource{path: "example.com/x/internal/tools/gadgets", source: `package gadgets

import gl "` + fixtureSDKPath + `"

type RunInput struct {
	Text string ~json:"text"~
}

func Run(c *gl.Client, in RunInput) error { return c.Widgets.Run(gl.Query{Text: &in.Text}) }
`},
		toolSource{path: "example.com/x/internal/tools/widgets", source: `package widgets

import gl "` + fixtureSDKPath + `"

type ListInput struct {
	Search string ~json:"search"~
}

func List(c *gl.Client, in ListInput) error {
	_, err := c.Widgets.ListWidgets(&gl.ListWidgetsOptions{Search: &in.Search})
	return err
}

type PermissionInput struct {
	UserID int64 ~json:"user_id"~
}

type ProtectInput struct {
	Name    string            ~json:"name"~
	Allowed []PermissionInput ~json:"allowed"~
}

func Protect(c *gl.Client, in ProtectInput) error {
	allowed := make([]*gl.PermissionOptions, 0, len(in.Allowed))
	for _, p := range in.Allowed {
		allowed = append(allowed, &gl.PermissionOptions{UserID: &p.UserID})
	}
	_ = gl.Query{}
	return c.Widgets.Protect(1, &gl.ProtectOptions{Name: &in.Name, Allowed: &allowed})
}
`})
	declare(t, acceptedMissingInputs, "widgets.ProtectInput.since", "a declared omission the export leaves out")

	var got []string
	for _, pair := range DiffInputPairs(pkgs) {
		got = append(got, pair.Package+" "+pair.MCPType+" "+pair.SDKNamed.Obj().Name()+" "+pair.Via+" "+strings.Join(pair.Missing, ","))
	}

	want := []string{
		"gadgets RunInput Query  ",
		"widgets ListInput ListWidgetsOptions  page",
		"widgets PermissionInput PermissionOptions nested level",
		"widgets ProtectInput ProtectOptions  by_user,levels,page",
	}
	if !slices.Equal(got, want) {
		t.Errorf("DiffInputPairs = %q, want %q", got, want)
	}
}

// clientGoPackage builds a package whose scope declares the Client struct,
// which is what identifies the client-go root.
func clientGoPackage(t *testing.T, path string, files []string) *packages.Package {
	t.Helper()
	typed := types.NewPackage(path, "gitlab")
	named := types.NewNamed(types.NewTypeName(token.NoPos, typed, "Client", nil), types.NewStruct(nil, nil), nil)
	typed.Scope().Insert(named.Obj())
	typed.MarkComplete()
	return &packages.Package{PkgPath: path, Types: typed, GoFiles: files}
}

// TestClientGoDir_AnImportGraph_IsResolvedToTheModuleRoot verifies how the SDK source is
// located. The import path carries a major-version suffix that moves with every
// major release and several client-go packages share its prefix, so the root is
// identified by the one declaration only it has. A wrong directory here parses
// no routes at all, which the join reads as an SDK nothing routes through.
func TestClientGoDir_AnImportGraph_IsResolvedToTheModuleRoot(t *testing.T) {
	const rootPath = shared.ClientGoPkgPath + "/v2"
	other := types.NewPackage(shared.ClientGoPkgPath+"/v2/testing", "testing")
	other.MarkComplete()

	cases := []struct {
		name    string
		imports map[string]*packages.Package
		want    string
	}{
		{
			// The fixture is spelled the way the host spells a path, since the
			// answer comes back through filepath.Dir and Windows separates with
			// a backslash.
			name:    "the root, among its siblings",
			imports: map[string]*packages.Package{rootPath: clientGoPackage(t, rootPath, []string{filepath.FromSlash("/cache/client-go/projects.go")})},
			want:    filepath.FromSlash("/cache/client-go"),
		},
		{name: "no imports at all"},
		{
			name:    "an import from somewhere else entirely",
			imports: map[string]*packages.Package{"net/http": {PkgPath: "net/http", GoFiles: []string{"/go/http.go"}}},
		},
		{
			name:    "a client-go package with no type information",
			imports: map[string]*packages.Package{rootPath: {PkgPath: rootPath, GoFiles: []string{"/cache/client-go/projects.go"}}},
		},
		{
			name:    "a client-go package that is not the root",
			imports: map[string]*packages.Package{other.Path(): {PkgPath: other.Path(), Types: other, GoFiles: []string{"/cache/client-go/testing/x.go"}}},
		},
		{
			name:    "the root with no files on disk",
			imports: map[string]*packages.Package{rootPath: clientGoPackage(t, rootPath, nil)},
		},
		{
			// The declaration is half the rule and the path is the other
			// half. This repository's own gitlab package declares a Client
			// struct too, and every tool package imports it, so a rule that
			// asked only for the declaration would resolve the SDK directory
			// to internal/gitlab and read no service method from it.
			name: "a Client struct outside the SDK module",
			imports: map[string]*packages.Package{
				"example.com/x/internal/gitlab": clientGoPackage(t, "example.com/x/internal/gitlab", []string{filepath.FromSlash("/repo/internal/gitlab/client.go")}),
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := ClientGoDir([]*packages.Package{{PkgPath: "tools", Imports: testCase.imports}})

			if got != testCase.want {
				t.Errorf("ClientGoDir() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestSDKNestedNames_AStruct_NamesWhatEachNestedClientGoStructDeserializes
// verifies the nested half of a pairing, which is what tells a key GitLab
// sends inside an object client-go already models from one it does not. A
// struct reached through a pointer, a slice or a map value is read; an
// untagged embed is descended into, below the struct's own fields, so a name
// both carry is the one encoding/json writes, the shallower; and a field
// whose type is no client-go struct, a client-go struct that deserializes
// nothing, a field no encoder writes and an embed of a type that is not a
// struct name nothing.
func TestSDKNestedNames_AStruct_NamesWhatEachNestedClientGoStructDeserializes(t *testing.T) {
	sdk := types.NewPackage(shared.ClientGoPkgPath+"/v3", "gitlab")
	pipeline := namedStruct(sdk, "Pipeline", makeStructWithTags(
		taggedField{name: "ID", tag: `json:"id"`, goType: types.Typ[types.Int64]},
		taggedField{name: "Name", tag: `json:"name"`, goType: types.Typ[types.String]},
	))
	user := namedStruct(sdk, "BasicUser", makeStructWithTags(taggedField{name: "Username", tag: `json:"username"`, goType: types.Typ[types.String]}))
	empty := namedStruct(sdk, "Empty", types.NewStruct(nil, nil))
	clock := namedStruct(types.NewPackage("example.com/clock", "clock"), "Instant", makeStructWithTags(taggedField{name: "Wall", tag: `json:"wall"`, goType: types.Typ[types.Int64]}))
	inner := namedStruct(sdk, "Basic", makeStructWithTags(
		taggedField{name: "Author", tag: `json:"author"`, goType: types.NewPointer(user)},
		taggedField{name: "HeadPipeline", tag: `json:"head_pipeline"`, goType: types.NewPointer(user)},
	))
	label := types.NewNamed(types.NewTypeName(token.NoPos, sdk, "Label", nil), types.Typ[types.String], nil)
	links := namedStruct(sdk, "Links", makeStructWithTags(taggedField{name: "Self", tag: `json:"self"`, goType: types.Typ[types.String]}))
	fields := []*types.Var{
		types.NewField(token.NoPos, sdk, "HeadPipeline", types.NewPointer(pipeline), false),
		types.NewField(token.NoPos, sdk, "Assignees", types.NewSlice(types.NewPointer(user)), false),
		types.NewField(token.NoPos, sdk, "Owners", types.NewMap(types.Typ[types.String], user), false),
		types.NewField(token.NoPos, sdk, "Nothing", empty, false),
		types.NewField(token.NoPos, sdk, "CreatedAt", types.NewPointer(clock), false),
		types.NewField(token.NoPos, sdk, "Title", types.Typ[types.String], false),
		types.NewField(token.NoPos, sdk, "hidden", types.NewPointer(pipeline), false),
		types.NewField(token.NoPos, sdk, "Skipped", types.NewPointer(pipeline), false),
		types.NewField(token.NoPos, sdk, "Basic", inner, true),
		types.NewField(token.NoPos, sdk, "Label", label, true),
		types.NewField(token.NoPos, sdk, "Links", links, true),
	}
	tags := []string{
		`json:"head_pipeline"`, `json:"assignees"`, `json:"owners"`, `json:"nothing"`, `json:"created_at"`,
		`json:"title"`, `json:"hidden"`, `json:"-"`, "", "", `json:"_links"`,
	}

	got := sdkNestedNames(types.NewStruct(fields, tags))

	// The embed tagged with a name is a field under that name, as encoding/json
	// writes it, and not a promotion.
	want := map[string][]string{
		"head_pipeline": {"id", "name"},
		"assignees":     {"username"},
		"owners":        {"username"},
		"author":        {"username"},
		"_links":        {"self"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sdkNestedNames() = %v, want %v", got, want)
	}
}

// TestSDKNestedNames_AStructNestingNothing_IsNil verifies the empty answer is
// nil rather than an empty map, so a pairing of a flat struct reads as one
// carrying no nested half.
func TestSDKNestedNames_AStructNestingNothing_IsNil(t *testing.T) {
	flat := makeStructWithTags(taggedField{name: "Title", tag: `json:"title"`, goType: types.Typ[types.String]})
	if got := sdkNestedNames(flat); got != nil {
		t.Errorf("sdkNestedNames(flat) = %v, want nil", got)
	}
}

// TestSDKNestedNames_AStructEmbeddingItself_IsWalkedOnce verifies the walk ends
// on a struct that embeds a pointer to itself, which encoding/json reads and
// the walk would otherwise descend into for ever, and that what it nests is
// still read.
func TestSDKNestedNames_AStructEmbeddingItself_IsWalkedOnce(t *testing.T) {
	sdk := types.NewPackage(shared.ClientGoPkgPath+"/v3", "gitlab")
	user := namedStruct(sdk, "BasicUser", makeStructWithTags(taggedField{name: "Username", tag: `json:"username"`, goType: types.Typ[types.String]}))
	node := types.NewNamed(types.NewTypeName(token.NoPos, sdk, "Node", nil), nil, nil)
	st := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, sdk, "Node", types.NewPointer(node), true),
		types.NewField(token.NoPos, sdk, "Author", types.NewPointer(user), false),
	}, []string{"", `json:"author"`})
	node.SetUnderlying(st)

	got := sdkNestedNames(st)

	if !reflect.DeepEqual(got, map[string][]string{"author": {"username"}}) {
		t.Errorf("sdkNestedNames() = %v, want the author read once", got)
	}
}
