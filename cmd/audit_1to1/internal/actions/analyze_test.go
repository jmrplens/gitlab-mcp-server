package actions

import (
	"encoding/json"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
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
			if got := shortPackage(input); got != want {
				t.Errorf("shortPackage(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestSortedSet_SortsAndDedups verifies set→sorted-slice conversion.
func TestSortedSet_SortsAndDedups(t *testing.T) {
	got := sortedSet(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedSet = %v, want %v", got, want)
	}
	if sortedSet(map[string]struct{}{}) == nil {
		t.Error("sortedSet of empty map should be non-nil empty slice")
	}
}

// TestSummarize_AggregatesServiceCounts verifies summary aggregation including
// the services-with-gaps tally.
func TestSummarize_AggregatesServiceCounts(t *testing.T) {
	s := summarize([]serviceCoverage{
		{Service: "A", APIMethods: 4, CoveredMethods: 4},
		{Service: "B", APIMethods: 6, CoveredMethods: 2, MissingMethods: []string{"X", "Y", "Z", "W"}},
	})
	if s.Services != 2 || s.ServicesWithGaps != 1 {
		t.Errorf("service counts = %d/%d, want 2/1", s.Services, s.ServicesWithGaps)
	}
	if s.APIMethods != 10 || s.CoveredMethods != 6 || s.MissingMethods != 4 {
		t.Errorf("method counts = %d/%d/%d, want 10/6/4", s.APIMethods, s.CoveredMethods, s.MissingMethods)
	}
}

// TestBuildReport_ResolvesSDKCoverage runs the auditor against the repository and
// asserts fix-agnostic invariants: every used service resolves API methods,
// coverage never exceeds the method count, and the output is sorted. It is the
// methodology regression guard for the SDK-method resolver.
func TestBuildReport_ResolvesSDKCoverage(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.Summary.APIMethods == 0 || rep.Summary.CoveredMethods == 0 {
		t.Fatalf("summary looks empty: %+v", rep.Summary)
	}
	var prev string
	for _, svc := range rep.Services {
		if svc.Service < prev {
			t.Errorf("services not sorted: %q before %q", prev, svc.Service)
		}
		prev = svc.Service
		if svc.CoveredMethods > svc.APIMethods {
			t.Errorf("service %s covered %d > api %d", svc.Service, svc.CoveredMethods, svc.APIMethods)
		}
		if svc.APIMethods == 0 {
			t.Errorf("service %s resolved zero API methods", svc.Service)
		}
		if len(svc.Packages) == 0 {
			t.Errorf("service %s has no referencing packages", svc.Service)
		}
	}
}

// TestBuildReport_Deterministic verifies repeated runs are byte-identical, a
// prerequisite for committing the report as a backlog.
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

// clientGoPkg is a synthetic package whose path passes the client-go check
// the resolver applies to receiver and option types.
var clientGoPkg = types.NewPackage(shared.ClientGoPkgPath+"/v2", "gitlab")

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
			if got := signatureIsAPICall(tc.sig); got != tc.want {
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
	if got := apiMethodNames(iface); !reflect.DeepEqual(got, []string{"CreateBranch", "ListBranches"}) {
		t.Errorf("apiMethodNames = %v, want the two exported endpoints sorted", got)
	}
	if got := apiMethodNames(namedIn(clientGoPkg, "Client", types.NewStruct(nil, nil))); got != nil {
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
			named, ok := clientGoServiceInterface(tc.typ)
			if ok != tc.want {
				t.Fatalf("clientGoServiceInterface = %v, want %v", ok, tc.want)
			}
			if ok && named != service {
				t.Errorf("returned %v, want the service interface", named)
			}
		})
	}
}

// TestCoverageForService_Usage_AdjudicatesAcceptedMethods verifies the
// per-service coverage: a called method and an adjudicated missing method
// both count as covered, the rest are the sorted missing list, and the
// referencing packages are listed sorted.
func TestCoverageForService_Usage_AdjudicatesAcceptedMethods(t *testing.T) {
	service := serviceInterface(clientGoPkg, "SearchServiceInterface",
		method(clientGoPkg, "Commits", true, param(requestOptionSlice())),
		method(clientGoPkg, "Milestones", true, param(requestOptionSlice())),
		method(clientGoPkg, "Blobs", true, param(requestOptionSlice())),
		method(clientGoPkg, "Users", true, param(requestOptionSlice())),
	)
	use := &serviceUsage{
		named:  service,
		called: map[string]struct{}{"Commits": {}},
		pkgs:   map[string]struct{}{"search": {}, "groups": {}},
	}
	got := coverageForService(use)
	want := serviceCoverage{
		Service:        "SearchServiceInterface",
		Packages:       []string{"groups", "search"},
		APIMethods:     4,
		CoveredMethods: 2,
		MissingMethods: []string{"Blobs", "Users"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("coverageForService = %+v, want %+v", got, want)
	}
}

// TestIsAcceptedMissingMethod_Keys_MatchServiceAndMethod verifies the
// adjudication lookup is keyed on the bare service name plus the method.
func TestIsAcceptedMissingMethod_Keys_MatchServiceAndMethod(t *testing.T) {
	cases := []struct {
		name    string
		service string
		method  string
		want    bool
	}{
		{name: "adjudicated_generic_search", service: "Search", method: "Milestones", want: true},
		{name: "adjudicated_graphql_epic", service: "Epics", method: "GetEpic", want: true},
		{name: "unlisted_method", service: "Search", method: "Blobs", want: false},
		{name: "service_suffix_is_not_part_of_the_key", service: "SearchServiceInterface", method: "Milestones", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAcceptedMissingMethod(tc.service, tc.method); got != tc.want {
				t.Errorf("isAcceptedMissingMethod(%q, %q) = %v, want %v", tc.service, tc.method, got, tc.want)
			}
		})
	}
}

// TestRun_Roots_EmitsJSONOrLoadError verifies the command-facing entry
// point: a root the loader cannot enter fails the run, and the repository
// root yields the report as indented JSON naming the client-go path.
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
	for _, svc := range rep.Services {
		if len(svc.MissingMethods) == 0 {
			t.Errorf("gaps-only report kept %s, which has no missing method", svc.Service)
		}
	}
}
