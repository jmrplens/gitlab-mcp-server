// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// pairingSDK is the part of client-go the request fixtures read: a service
// whose methods take an Options struct, one that reaches two nested structs
// through a pointer to a slice, a map and an array, and a struct under
// another name; an embedded struct, an upload's content, a value type with no
// exported field and a field that reaches its own struct, none of which is a
// nested struct of parameters; and the GraphQL interface, which is not a
// service. A tilde stands for a backtick, so the tags read as they do in the
// SDK.
const pairingSDK = `package gitlab

import "io"

type RequestOptionFunc func()

type ListOptions struct {
	Page int64 ~url:"page,omitempty" json:"page,omitempty"~
}

type ISOTime struct {
	seconds int64
}

type Avatar struct {
	Filename string
	Image    io.Reader
}

type Level struct {
	Value *int64 ~url:"value,omitempty" json:"value,omitempty"~
}

type PermissionOptions struct {
	UserID *int64 ~url:"user_id,omitempty" json:"user_id,omitempty"~
	Level  *Level ~url:"level,omitempty" json:"level,omitempty"~
}

type ProtectOptions struct {
	ListOptions
	Name    *string                       ~url:"name,omitempty" json:"name,omitempty"~
	Allowed *[]*PermissionOptions         ~url:"allowed,omitempty" json:"allowed,omitempty"~
	ByUser  map[string]*PermissionOptions ~url:"by_user,omitempty" json:"by_user,omitempty"~
	Levels  [2]*Level                     ~url:"levels,omitempty" json:"levels,omitempty"~
	Avatar  *Avatar                       ~url:"-" json:"-"~
	Since   *ISOTime                      ~url:"since,omitempty" json:"since,omitempty"~
	Parent  *ProtectOptions               ~url:"-" json:"-"~
}

type ListWidgetsOptions struct {
	ListOptions
	Search *string ~url:"search,omitempty" json:"search,omitempty"~
}

type Query struct {
	Text *string ~json:"text,omitempty"~
}

type Widget struct {
	ID int64 ~json:"id"~
}

type LegacyServiceInterface struct{}

type WidgetsServiceInterface interface {
	ListWidgets(opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, error)
	Protect(pid any, opt *ProtectOptions, options ...RequestOptionFunc) error
	Run(query Query, options ...RequestOptionFunc) error
}

type GraphQLQuery struct {
	Query string ~json:"query"~
}

type GraphQLInterface interface {
	Do(query GraphQLQuery, target any, options ...RequestOptionFunc) error
}

type Client struct {
	Widgets WidgetsServiceInterface
	GraphQL GraphQLInterface
}
`

// toolSource is one tool package of a fixture tree: its import path and the
// source of its one file, in which a tilde stands for a backtick.
type toolSource struct {
	path   string
	source string
}

// backticked turns the fixtures' tildes into the backticks Go tags are
// quoted with.
func backticked(source string) string { return strings.ReplaceAll(source, "~", "`") }

// checkedTree type-checks sdk at [fixtureSDKPath] and then each tool package
// in order, and returns the tool packages the way go/packages hands them to
// the audit: with their syntax, their type information, their file set and
// the tool packages they import among their imports. A package imports only
// the SDK and the packages listed before it.
func checkedTree(t *testing.T, sdk string, tools ...toolSource) []*packages.Package {
	t.Helper()
	fileSet := token.NewFileSet()
	checked := map[string]*packages.Package{}
	importFrom := importerFunc(func(path string) (*types.Package, error) {
		if pkg, isChecked := checked[path]; isChecked {
			return pkg.Types, nil
		}
		return importer.Default().Import(path)
	})
	check := func(path, filename, source string) *packages.Package {
		file, err := parser.ParseFile(fileSet, filename, backticked(source), 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		info := &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		typed, err := (&types.Config{Importer: importFrom}).Check(path, fileSet, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("check %s: %v", path, err)
		}
		pkg := &packages.Package{PkgPath: path, Fset: fileSet, Types: typed, TypesInfo: info, Syntax: []*ast.File{file}, Imports: map[string]*packages.Package{}}
		for _, imported := range typed.Imports() {
			if dep, isChecked := checked[imported.Path()]; isChecked {
				pkg.Imports[imported.Path()] = dep
			}
		}
		checked[path] = pkg
		return pkg
	}
	check(fixtureSDKPath, "sdk.go", sdk)
	out := make([]*packages.Package, 0, len(tools))
	for _, tool := range tools {
		out = append(out, check(tool.path, tool.path[strings.LastIndex(tool.path, "/")+1:]+".go", tool.source))
	}
	return out
}

// fixtureSDKOf returns the SDK package a checked tree compiled against.
func fixtureSDKOf(t *testing.T, pkgs []*packages.Package) *types.Package {
	t.Helper()
	for _, pkg := range pkgs {
		for path, imported := range pkg.Imports {
			if path == fixtureSDKPath {
				return imported.Types
			}
		}
	}
	t.Fatal("the fixture tree imports no SDK")
	return nil
}

// sdkTypeNamed looks a named type up in the fixture SDK.
func sdkTypeNamed(t *testing.T, sdk *types.Package, name string) *types.Named {
	t.Helper()
	obj, isType := sdk.Scope().Lookup(name).(*types.TypeName)
	if !isType {
		t.Fatalf("the fixture SDK declares no type %s", name)
	}
	named, _ := obj.Type().(*types.Named)
	return named
}

// namesOf lists the names of a set of type names, sorted.
func namesOf(set map[*types.TypeName]bool) []string {
	out := make([]string, 0, len(set))
	for obj, member := range set {
		if member {
			out = append(out, obj.Name())
		}
	}
	slices.Sort(out)
	return out
}

// sdkOnlyTree checks the SDK alone, behind a tool package that imports it.
func sdkOnlyTree(t *testing.T, sdk string) *types.Package {
	t.Helper()
	pkgs := checkedTree(t, sdk, toolSource{path: "example.com/x/internal/tools/widgets", source: `package widgets

import gl "` + fixtureSDKPath + `"

var _ gl.Client
`})
	return fixtureSDKOf(t, pkgs)
}

// TestRequestStructsOf_AStandInSDK_SeparatesWhatMethodsTakeFromWhatTheyReach
// verifies the request universe read off the SDK: a struct a service method
// takes is received, whether it is spelled as Options or not and whether it
// is passed by pointer or by value, and a struct a received one reaches
// through a field is nested. Neither set holds the GraphQL document, which
// the GraphQL interface takes and which is no service, a result struct, an
// embedded struct, an upload's content or a value type, and a type named like
// a service that is no interface adds nothing. The set is read once per SDK.
func TestRequestStructsOf_AStandInSDK_SeparatesWhatMethodsTakeFromWhatTheyReach(t *testing.T) {
	sdk := sdkOnlyTree(t, pairingSDK)

	requests := RequestStructsOf(sdk)

	if got, want := namesOf(requests.Received), []string{"ListWidgetsOptions", "ProtectOptions", "Query"}; !slices.Equal(got, want) {
		t.Errorf("Received = %v, want %v", got, want)
	}
	if got, want := namesOf(requests.Nested), []string{"Level", "PermissionOptions"}; !slices.Equal(got, want) {
		t.Errorf("Nested = %v, want %v", got, want)
	}
	if RequestStructsOf(sdk) != requests {
		t.Error("RequestStructsOf read the SDK a second time instead of answering from its cache")
	}
}

// TestNestedStructs_ARequest_ListsWhatItsFieldsReachOnce verifies the walk a
// rule holding nested structs to inputs reads: every struct of parameters a
// request reaches, in the order its fields reach them and each once, the
// struct itself never, however a field wraps the struct it reaches.
func TestNestedStructs_ARequest_ListsWhatItsFieldsReachOnce(t *testing.T) {
	sdk := sdkOnlyTree(t, pairingSDK)

	var got []string
	for _, nested := range NestedStructs(sdkTypeNamed(t, sdk, "ProtectOptions")) {
		got = append(got, nested.Obj().Name())
	}

	if want := []string{"PermissionOptions", "Level"}; !slices.Equal(got, want) {
		t.Errorf("NestedStructs(ProtectOptions) = %v, want %v", got, want)
	}
	if nested := NestedStructs(sdkTypeNamed(t, sdk, "Level")); len(nested) != 0 {
		t.Errorf("NestedStructs(Level) = %v, want none for a struct of scalars", nested)
	}
}

// TestNestedStructs_AChainDeeperThanTheBound_StopsAtIt verifies the walk ends
// at the depth the field flattening ends at, whether each level reaches the
// next through a field or embeds it: a chain of fields is followed seven
// levels and no further, a struct embedded deeper than that is not entered,
// and one embedded two levels down is.
func TestNestedStructs_AChainDeeperThanTheBound_StopsAtIt(t *testing.T) {
	var chain strings.Builder
	chain.WriteString("package gitlab\n\ntype Client struct{}\n\ntype Leaf struct {\n\tValue int64 ~json:\"value\"~\n}\n")
	for level := range 9 {
		digit, next := string(rune('0'+level)), string(rune('1'+level))
		chain.WriteString("\ntype Level" + digit + " struct {\n\tValue int64 ~json:\"value\"~\n")
		embedded := "\tLeaf *Leaf ~json:\"leaf\"~\n"
		if level < 8 {
			chain.WriteString("\tNext *Level" + next + " ~json:\"next\"~\n")
			embedded = "\tEmbedded" + next + "\n"
		}
		chain.WriteString("}\n\ntype Embedded" + digit + " struct {\n" + embedded + "}\n")
	}
	chain.WriteString("\ntype Shallow struct {\n\tEmbedded7\n}\n")
	sdk := sdkOnlyTree(t, chain.String())
	names := func(root string) []string {
		var out []string
		for _, nested := range NestedStructs(sdkTypeNamed(t, sdk, root)) {
			out = append(out, nested.Obj().Name())
		}
		return out
	}

	if got, want := names("Level0"), []string{"Level1", "Level2", "Level3", "Level4", "Level5", "Level6", "Level7"}; !slices.Equal(got, want) {
		t.Errorf("NestedStructs(Level0) = %v, want the seven levels the bound reaches", got)
	}
	if got := names("Embedded0"); len(got) != 0 {
		t.Errorf("NestedStructs(Embedded0) = %v, want nothing past eight embedded levels", got)
	}
	if got, want := names("Shallow"), []string{"Leaf"}; !slices.Equal(got, want) {
		t.Errorf("NestedStructs(Shallow) = %v, want %v", got, want)
	}
}

// TestReceivedStructs_ASignature_ListsItsStructParametersButTheVariadicTail
// verifies which parameters of a method are what it takes: each one whose
// type is a client-go struct or a pointer to one, in order, and never the
// variadic tail, which is a list of request options even when its element is
// a struct.
func TestReceivedStructs_ASignature_ListsItsStructParametersButTheVariadicTail(t *testing.T) {
	sdk := sdkOnlyTree(t, pairingSDK)
	param := func(name string, typ types.Type) *types.Var { return types.NewParam(token.NoPos, nil, name, typ) }
	options := sdkTypeNamed(t, sdk, "ListWidgetsOptions")
	query := sdkTypeNamed(t, sdk, "Query")
	level := sdkTypeNamed(t, sdk, "Level")
	cases := []struct {
		name string
		sig  *types.Signature
		want []string
	}{
		{
			name: "a pointer and a value, around a scalar",
			sig: types.NewSignatureType(nil, nil, nil, types.NewTuple(
				param("opt", types.NewPointer(options)), param("pid", types.Typ[types.Int64]), param("query", query),
			), nil, false),
			want: []string{"ListWidgetsOptions", "Query"},
		},
		{
			name: "a variadic tail of structs",
			sig: types.NewSignatureType(nil, nil, nil, types.NewTuple(
				param("query", query), param("rest", types.NewSlice(types.NewPointer(level))),
			), nil, true),
			want: []string{"Query"},
		},
		{
			name: "a struct that is the only, variadic, parameter",
			sig:  types.NewSignatureType(nil, nil, nil, types.NewTuple(param("rest", types.NewSlice(level))), nil, true),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, received := range ReceivedStructs(tc.sig) {
				got = append(got, received.Obj().Name())
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ReceivedStructs = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRequestPredicates_EachStruct_IsClassifiedByWhereTheRequestCarriesIt
// verifies the four questions the pairing asks of a struct: whether a
// literal of it is a request a handler pairs with its input, whether it is
// nested, whether a request carries it at all, and whether it carries an
// upload's content; and the one it asks of the other side, whether a struct
// is declared in a tool package.
func TestRequestPredicates_EachStruct_IsClassifiedByWhereTheRequestCarriesIt(t *testing.T) {
	sdk := sdkOnlyTree(t, pairingSDK)
	cases := []struct {
		name                             string
		literal, nested, request, upload bool
	}{
		{name: "ListWidgetsOptions", literal: true, request: true},
		{name: "Query", literal: true, request: true},
		{name: "ListOptions", literal: true},
		{name: "PermissionOptions", nested: true, request: true},
		{name: "Level", nested: true, request: true},
		{name: "Widget"},
		{name: "Avatar", upload: true},
		{name: "GraphQLQuery"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named := sdkTypeNamed(t, sdk, tc.name)
			st, _ := named.Underlying().(*types.Struct)
			got := []bool{requestLiteral(named), isNestedRequest(named), IsRequestStruct(named), carriesReader(st)}
			if want := []bool{tc.literal, tc.nested, tc.request, tc.upload}; !reflect.DeepEqual(got, want) {
				t.Errorf("literal, nested, request, upload = %v, want %v", got, want)
			}
		})
	}

	t.Run("a tool package's struct and anything else", func(t *testing.T) {
		tool := namedStruct(types.NewPackage("example.com/x/internal/tools/widgets", "widgets"), "ListInput", makeStruct())
		other := namedStruct(types.NewPackage("example.com/x/internal/toolutil", "toolutil"), "PaginationInput", makeStruct())
		universe := types.NewNamed(types.NewTypeName(token.NoPos, nil, "error", nil), makeStruct(), nil)
		if !isToolStruct(tool) || isToolStruct(other) || isToolStruct(universe) {
			t.Errorf("isToolStruct = %v/%v/%v, want true only for the tool package's struct", isToolStruct(tool), isToolStruct(other), isToolStruct(universe))
		}
	})
}

// embeddedRequestSource builds the list options inside a struct of the
// tool package's own that embeds them beside a parameter client-go does not
// model, a query held in a field and a result embedded, by a handler and by a
// helper.
const embeddedRequestSource = `package widgets

import gl "` + fixtureSDKPath + `"

type body struct {
	gl.ListWidgetsOptions
	gl.Widget
	Query gl.Query
	Extra *bool ~url:"extra,omitempty" json:"extra,omitempty"~
}

type BodyInput struct {
	Search string ~json:"search"~
	Extra  *bool  ~json:"extra"~
}

// Body fills the body field by field and sends the options it embeds.
func Body(c *gl.Client, in BodyInput) error {
	opts := &body{Extra: in.Extra}
	opts.Search = &in.Search
	_, err := c.Widgets.ListWidgets(&opts.ListWidgetsOptions)
	return err
}

// newBody builds the body for its caller.
func newBody(extra *bool) *body { return &body{Extra: extra} }

type HelperBodyInput struct {
	Extra *bool ~json:"extra"~
}

// HelperBody has the helper build the body.
func HelperBody(c *gl.Client, in HelperBodyInput) error {
	_, err := c.Widgets.ListWidgets(&newBody(in.Extra).ListWidgetsOptions)
	return err
}
`

// TestBuiltRequests_AStructThatEmbedsARequest_BuildsTheRequest verifies a
// literal of a struct of our own that embeds a client-go request builds that
// request, in a handler and in a helper its caller hands its input: the
// request is paired with the input, and neither the query held in a field
// nor the result embedded beside it is.
func TestBuiltRequests_AStructThatEmbedsARequest_BuildsTheRequest(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK, toolSource{path: "example.com/x/internal/tools/widgets", source: embeddedRequestSource})

	inputs, _ := discoverPairs(pkgs[0])

	var got []string
	for key, pair := range inputs {
		got = append(got, key[0]+" "+key[1]+" "+pair.via)
	}
	slices.Sort(got)
	if want := []string{"BodyInput ListWidgetsOptions ", "HelperBodyInput ListWidgetsOptions caller"}; !slices.Equal(got, want) {
		t.Errorf("discoverPairs = %q, want %q", got, want)
	}
}
