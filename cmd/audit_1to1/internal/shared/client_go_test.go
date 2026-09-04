// Tests for the client-go type resolution the audit scopes share: which
// receiver is a service interface, which method is a REST endpoint, and how a
// package path reduces to a domain name.

package shared

import (
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"
)

// TestShortPackage_ExtractsDomain verifies the internal/tools domain extraction
// and its fallbacks for paths outside the tools tree.
func TestShortPackage_ExtractsDomain(t *testing.T) {
	cases := map[string]string{
		"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/branches": "branches",
		"github.com/x/internal/tools/group/sub":                            "group/sub",
		"flat":                                                             "flat",
		"a/b/c":                                                            "c",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := ShortPackage(input); got != want {
				t.Errorf("ShortPackage(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestSortedSet_SortsAndDedups verifies set→sorted-slice conversion.
func TestSortedSet_SortsAndDedups(t *testing.T) {
	got := SortedSet(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedSet = %v, want %v", got, want)
	}
	if SortedSet(map[string]struct{}{}) == nil {
		t.Error("sortedSet of empty map should be non-nil empty slice")
	}
}

// clientGoPkg is a synthetic package whose path passes the client-go check
// the resolver applies to receiver and option types.
var clientGoPkg = types.NewPackage(ClientGoPkgPath+"/v2", "gitlab")

// namedIn declares a named type in pkg over underlying.
func namedIn(pkg *types.Package, name string, underlying types.Type) *types.Named {
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), underlying, nil)
}

// requestOptionSlice is the ...RequestOptionFunc variadic tail of a client-go
// API method.
func requestOptionSlice() *types.Slice {
	return types.NewSlice(namedIn(clientGoPkg, "RequestOptionFunc", types.NewSignatureType(nil, nil, nil, nil, nil, false)))
}

// method builds an interface method with the given parameters.
func method(pkg *types.Package, name string, variadic bool, params ...*types.Var) *types.Func {
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), nil, variadic)
	return types.NewFunc(token.NoPos, pkg, name, sig)
}

// param builds one unnamed parameter of type typ.
func param(typ types.Type) *types.Var {
	return types.NewParam(token.NoPos, clientGoPkg, "", typ)
}

// serviceInterface declares a client-go service interface with methods.
func serviceInterface(pkg *types.Package, name string, methods ...*types.Func) *types.Named {
	iface := types.NewInterfaceType(methods, nil)
	iface.Complete()
	return namedIn(pkg, name, iface)
}

// TestSignatureIsAPICall_Shapes_RecognizesOnlyRequestOptionTails verifies
// the REST endpoint marker: only a variadic signature whose tail is a slice
// of a client-go type named after RequestOptionFunc counts, so plain
// variadics, named slice types, unnamed or foreign element types and other
// client-go types are all rejected.
func TestSignatureIsAPICall_Shapes_RecognizesOnlyRequestOptionTails(t *testing.T) {
	otherPkg := types.NewPackage("example.com/other", "other")
	cases := []struct {
		name string
		sig  *types.Signature
		want bool
	}{
		{name: "no_params", sig: method(clientGoPkg, "M", false).Signature(), want: false},
		{name: "not_variadic", sig: method(clientGoPkg, "M", false, param(requestOptionSlice())).Signature(), want: false},
		{
			name: "named_slice_type_is_not_a_bare_slice",
			sig:  method(clientGoPkg, "M", true, param(namedIn(clientGoPkg, "Options", requestOptionSlice()))).Signature(),
			want: false,
		},
		{name: "unnamed_element", sig: method(clientGoPkg, "M", true, param(types.NewSlice(types.Typ[types.String]))).Signature(), want: false},
		{name: "universe_element_has_no_package", sig: method(clientGoPkg, "M", true, param(types.NewSlice(types.Universe.Lookup("error").Type()))).Signature(), want: false},
		{
			name: "foreign_package_element",
			sig:  method(clientGoPkg, "M", true, param(types.NewSlice(namedIn(otherPkg, "RequestOptionFunc", types.Typ[types.Int])))).Signature(),
			want: false,
		},
		{
			name: "client_go_element_without_marker",
			sig:  method(clientGoPkg, "M", true, param(types.NewSlice(namedIn(clientGoPkg, "Client", types.Typ[types.Int])))).Signature(),
			want: false,
		},
		{name: "request_option_tail", sig: method(clientGoPkg, "M", true, param(types.Typ[types.Int]), param(requestOptionSlice())).Signature(), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SignatureIsAPICall(tc.sig); got != tc.want {
				t.Errorf("signatureIsAPICall = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAPIMethodNames_Interface_ListsExportedEndpointsSorted verifies the
// endpoint listing keeps only the exported methods with the REST marker, in
// sorted order, and yields nothing for a named type that is not an interface.
func TestAPIMethodNames_Interface_ListsExportedEndpointsSorted(t *testing.T) {
	iface := serviceInterface(clientGoPkg, "BranchesServiceInterface",
		method(clientGoPkg, "ListBranches", true, param(requestOptionSlice())),
		method(clientGoPkg, "CreateBranch", true, param(requestOptionSlice())),
		method(clientGoPkg, "helper", true, param(requestOptionSlice())),
		method(clientGoPkg, "String", false),
	)
	if got := APIMethodNames(iface); !reflect.DeepEqual(got, []string{"CreateBranch", "ListBranches"}) {
		t.Errorf("apiMethodNames = %v, want the two exported endpoints sorted", got)
	}
	if got := APIMethodNames(namedIn(clientGoPkg, "Client", types.NewStruct(nil, nil))); got != nil {
		t.Errorf("apiMethodNames of a struct = %v, want nil", got)
	}
}

// TestClientGoServiceInterface_Types_AcceptsOnlyClientGoInterfaces verifies
// the receiver test behind every recorded call: a client-go interface,
// reached directly or through a pointer, is a service; nil, basic, struct,
// universe and foreign-package types are not.
func TestClientGoServiceInterface_Types_AcceptsOnlyClientGoInterfaces(t *testing.T) {
	service := serviceInterface(clientGoPkg, "TagsServiceInterface", method(clientGoPkg, "ListTags", true, param(requestOptionSlice())))
	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "nil", typ: nil, want: false},
		{name: "basic", typ: types.Typ[types.Int], want: false},
		{name: "client_go_struct", typ: namedIn(clientGoPkg, "Tag", types.NewStruct(nil, nil)), want: false},
		{name: "universe_error_interface", typ: types.Universe.Lookup("error").Type(), want: false},
		{name: "foreign_interface", typ: serviceInterface(types.NewPackage("example.com/x", "x"), "Svc"), want: false},
		{name: "service_interface", typ: service, want: true},
		{name: "pointer_to_service_interface", typ: types.NewPointer(service), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named, ok := ClientGoServiceInterface(tc.typ)
			if ok != tc.want {
				t.Fatalf("clientGoServiceInterface = %v, want %v", ok, tc.want)
			}
			if ok && named != service {
				t.Errorf("returned %v, want the service interface", named)
			}
		})
	}
}

// TestServiceName_Interfaces_StripOnlyTheSuffix verifies the bare service name
// both adjudication tables are keyed on: the ServiceInterface suffix goes, and
// an interface without it (GraphQLInterface) keeps its whole name.
func TestServiceName_Interfaces_StripOnlyTheSuffix(t *testing.T) {
	cases := map[string]string{
		"BranchesServiceInterface": "Branches",
		"GraphQLInterface":         "GraphQLInterface",
		"ServiceInterface":         "",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ServiceName(serviceInterface(clientGoPkg, name)); got != want {
				t.Errorf("ServiceName(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// fixtureSDKSource is the stand-in client-go the loader-backed cases resolve.
// Its import path carries the client-go path as an infix, which is what the
// resolver matches on.
const fixtureSDKSource = `package gitlab

type RequestOptionFunc func()

type BranchesServiceInterface interface {
	ListBranches(options ...RequestOptionFunc) error
	CreateBranch(options ...RequestOptionFunc) error
}

type GraphQLInterface interface {
	Do(query string, target any, options ...RequestOptionFunc) error
}

type Client struct {
	GraphQL  GraphQLInterface
	Branches BranchesServiceInterface
}
`

// sdkPath is where a fixture module keeps its stand-in client-go.
const sdkPath = "gitlab.com/gitlab-org/api/client-go/v2/sdk.go"

// sdkImport is the import line a fixture tool package uses to reach it.
const sdkImport = "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v2"

// sdkModule writes a throwaway module whose internal/tools tree calls the
// stand-in client-go, and returns its root.
func sdkModule(t *testing.T, sdk string) string {
	t.Helper()
	return writeModule(t, map[string]string{
		sdkPath: sdk,
		"internal/tools/branches/branches.go": "package branches\n\nimport gl \"" + sdkImport + "\"\n\n" +
			"// List calls one endpoint and leaves the other uncalled.\nfunc List(c *gl.Client) error { return c.Branches.ListBranches() }\n",
	})
}

// TestClientGoTypes_ImportGraph_FindsTheRootPackage verifies the SDK types are
// read out of the loaded tool packages' import graph, so the surface audited is
// the one the handlers compile against, and that a tree importing no client-go
// fails rather than reporting an empty universe.
func TestClientGoTypes_ImportGraph_FindsTheRootPackage(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		pkgs, err := LoadToolPackages(sdkModule(t, fixtureSDKSource))
		if err != nil {
			t.Fatalf("LoadToolPackages: %v", err)
		}
		clientGo, err := ClientGoTypes(pkgs)
		if err != nil {
			t.Fatalf("ClientGoTypes: %v", err)
		}
		if clientGo.Scope().Lookup("Client") == nil {
			t.Error("resolved a package without a Client struct")
		}
	})

	t.Run("absent", func(t *testing.T) {
		pkgs, err := LoadToolPackages(writeModule(t, map[string]string{
			"internal/tools/alpha/alpha.go": "package alpha\n\nfunc A() int { return 1 }\n",
		}))
		if err != nil {
			t.Fatalf("LoadToolPackages: %v", err)
		}
		if _, typesErr := ClientGoTypes(pkgs); typesErr == nil || !strings.Contains(typesErr.Error(), "client-go root package not found") {
			t.Fatalf("ClientGoTypes = %v, want a not-found error", typesErr)
		}
	})
}

// TestClientStructAndGraphQLInterface_Shapes_RequireTheRealDeclarations
// verifies both structural lookups resolve the real declarations, and refuse a
// package that renames or reshapes them rather than returning an empty result
// that would read as a passing audit.
func TestClientStructAndGraphQLInterface_Shapes_RequireTheRealDeclarations(t *testing.T) {
	pkgs, err := LoadToolPackages(sdkModule(t, fixtureSDKSource))
	if err != nil {
		t.Fatalf("LoadToolPackages: %v", err)
	}
	clientGo, err := ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("ClientGoTypes: %v", err)
	}
	st, err := ClientStruct(clientGo)
	if err != nil {
		t.Fatalf("ClientStruct: %v", err)
	}
	if st.NumFields() != 2 {
		t.Errorf("Client has %d fields, want the 2 declared", st.NumFields())
	}
	graphQL, err := GraphQLInterface(clientGo)
	if err != nil {
		t.Fatalf("GraphQLInterface: %v", err)
	}
	if graphQL.Obj().Name() != "GraphQLInterface" {
		t.Errorf("GraphQLInterface resolved %q", graphQL.Obj().Name())
	}

	// Each stand-in below keeps a Client struct so the root package still
	// resolves, and breaks exactly one of the two lookups.
	cases := []struct {
		name    string
		sdk     string
		wantErr string
	}{
		{
			name:    "graphql_renamed_away",
			sdk:     "package gitlab\n\ntype Client struct{}\n",
			wantErr: "GraphQLInterface not found",
		},
		{
			name:    "graphql_is_not_a_named_type",
			sdk:     "package gitlab\n\ntype Client struct{}\n\nfunc GraphQLInterface() {}\n",
			wantErr: "GraphQLInterface is not a named type",
		},
		{
			name:    "graphql_names_a_struct",
			sdk:     "package gitlab\n\ntype Client struct{}\n\ntype GraphQLInterface struct{}\n",
			wantErr: "GraphQLInterface is not an interface",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeModule(t, map[string]string{
				sdkPath: tc.sdk,
				"internal/tools/alpha/alpha.go": "package alpha\n\nimport gl \"" + sdkImport + "\"\n\n" +
					"// Use keeps the SDK in the import graph.\nfunc Use(c *gl.Client) *gl.Client { return c }\n",
			})
			loaded, loadErr := LoadToolPackages(root)
			if loadErr != nil {
				t.Fatalf("LoadToolPackages: %v", loadErr)
			}
			pkg, typesErr := ClientGoTypes(loaded)
			if typesErr != nil {
				t.Fatalf("ClientGoTypes: %v", typesErr)
			}
			if _, gqlErr := GraphQLInterface(pkg); gqlErr == nil || !strings.Contains(gqlErr.Error(), tc.wantErr) {
				t.Errorf("GraphQLInterface error = %v, want it to contain %q", gqlErr, tc.wantErr)
			}
		})
	}

	t.Run("client_is_not_a_struct", func(t *testing.T) {
		// A Client that is not a struct makes the whole root package
		// unresolvable, which is how a renamed entry point surfaces.
		root := writeModule(t, map[string]string{
			sdkPath: "package gitlab\n\ntype Client interface{}\n",
			"internal/tools/alpha/alpha.go": "package alpha\n\nimport gl \"" + sdkImport + "\"\n\n" +
				"// Use keeps the SDK in the import graph.\nfunc Use(c gl.Client) gl.Client { return c }\n",
		})
		loaded, loadErr := LoadToolPackages(root)
		if loadErr != nil {
			t.Fatalf("LoadToolPackages: %v", loadErr)
		}
		if _, typesErr := ClientGoTypes(loaded); typesErr == nil || !strings.Contains(typesErr.Error(), "client-go root package not found") {
			t.Fatalf("ClientGoTypes = %v, want a not-found error", typesErr)
		}
	})
}

// TestCollectServiceUsage_CallSites_RecordMethodsAndPackages verifies the
// call-site universe both scopes read: the interface a call goes through is
// keyed by its own name, only the methods actually called are recorded, and the
// referencing package is named by its internal/tools domain.
func TestCollectServiceUsage_CallSites_RecordMethodsAndPackages(t *testing.T) {
	pkgs, err := LoadToolPackages(sdkModule(t, fixtureSDKSource))
	if err != nil {
		t.Fatalf("LoadToolPackages: %v", err)
	}
	usage := CollectServiceUsage(pkgs)
	use, ok := usage["BranchesServiceInterface"]
	if !ok {
		t.Fatalf("Branches not recorded (got %v)", usage)
	}
	if use.Service() != "Branches" {
		t.Errorf("Service() = %q, want Branches", use.Service())
	}
	if _, called := use.Called["ListBranches"]; !called {
		t.Errorf("called = %v, want ListBranches", use.Called)
	}
	if _, called := use.Called["CreateBranch"]; called {
		t.Error("CreateBranch recorded as called, but nothing calls it")
	}
	if !reflect.DeepEqual(SortedSet(use.Packages), []string{"branches"}) {
		t.Errorf("packages = %v, want [branches]", SortedSet(use.Packages))
	}
	if _, recorded := usage["GraphQLInterface"]; recorded {
		t.Error("GraphQLInterface recorded although no handler calls it")
	}
}

// TestClientStruct_Package_RefusesANonStructClient verifies the struct lookup
// itself, given a package whose Client is not a struct. The loader-backed cases
// above cannot reach this branch, because such a package never resolves as the
// client-go root in the first place.
func TestClientStruct_Package_RefusesANonStructClient(t *testing.T) {
	pkg := types.NewPackage(ClientGoPkgPath+"/v2", "gitlab")
	iface := types.NewInterfaceType(nil, nil)
	iface.Complete()
	named := namedIn(pkg, "Client", iface)
	pkg.Scope().Insert(named.Obj())
	if _, err := ClientStruct(pkg); err == nil || !strings.Contains(err.Error(), "Client struct not found") {
		t.Fatalf("ClientStruct = %v, want a not-found error", err)
	}
}
